package wxpay

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// refundPath 是微信支付 V3 申请退款接口。
const refundPath = "/v3/refund/domestic/refunds"

// 微信退款单状态（refund_status）。
const (
	RefundStatusSuccess    = "SUCCESS"
	RefundStatusProcessing = "PROCESSING"
	RefundStatusClosed     = "CLOSED"
	RefundStatusAbnormal   = "ABNORMAL"
)

// RefundRequest 是一次退款申请（与订单业务解耦，out_refund_no 为幂等键）。
type RefundRequest struct {
	OutTradeNo  string // 原商户订单号
	OutRefundNo string // 商户退款单号（本项目以退款工单号充当，重试复用）
	Reason      string // 退款原因（展示给用户，可空）
	AmountFen   int64  // 退款金额（分）
	TotalFen    int64  // 原订单金额（分）
}

// RefundResult 是退款申请的受理结果。
type RefundResult struct {
	RefundID    string // 微信退款单号（payments.refund_id 幂等键）
	OutRefundNo string
	OutTradeNo  string
	Status      string // SUCCESS / PROCESSING ...
}

// Refunder 是退款申请能力（真实通道与 mock 均实现）。
type Refunder interface {
	Refund(ctx context.Context, req RefundRequest) (*RefundResult, error)
}

// RefundNotifyResult 是退款结果回调解密后的结果。
type RefundNotifyResult struct {
	MchID         string
	OutTradeNo    string
	TransactionID string
	OutRefundNo   string
	RefundID      string
	RefundStatus  string
	SuccessTime   string
	RefundAmount  int64 // 退款金额（分）
	TotalAmount   int64 // 原订单金额（分）
}

// RefundNotifier 解析并验签微信退款结果回调。
type RefundNotifier interface {
	ParseRefundNotify(headers NotifyHeaders, body []byte) (*RefundNotifyResult, error)
}

// Refund 申请原路退款（传输层失败重试一次；微信业务错误不重试）。
// 以 out_refund_no 幂等：同一单号重试由微信返回原退款单。
func (c *Client) Refund(ctx context.Context, req RefundRequest) (*RefundResult, error) {
	var lastErr error
	for attempt := 0; attempt <= maxHTTPRetries; attempt++ {
		result, err := c.doRefund(ctx, req)
		if err == nil {
			return result, nil
		}
		var wechatErr *Error
		if errors.As(err, &wechatErr) {
			return nil, err
		}
		lastErr = err
	}
	return nil, fmt.Errorf("wxpay refund: %w", lastErr)
}

// doRefund 执行一次退款请求并解析响应。
func (c *Client) doRefund(ctx context.Context, req RefundRequest) (*RefundResult, error) {
	if req.OutTradeNo == "" || req.OutRefundNo == "" {
		return nil, errors.New("wxpay refund: out_trade_no/out_refund_no 不能为空")
	}
	if req.AmountFen <= 0 || req.TotalFen <= 0 || req.AmountFen > req.TotalFen {
		return nil, errors.New("wxpay refund: 退款金额必须大于 0 且不超过订单金额")
	}
	body := map[string]any{
		"out_trade_no":  req.OutTradeNo,
		"out_refund_no": req.OutRefundNo,
		"notify_url":    c.refundNotifyURL,
		"amount": map[string]any{
			"refund":   req.AmountFen,
			"total":    req.TotalFen,
			"currency": "CNY",
		},
	}
	if req.Reason != "" {
		body["reason"] = req.Reason
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal refund body: %w", err)
	}

	timestamp, nonce, signature, err := c.signRequest(http.MethodPost, refundPath, raw)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+refundPath, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("build refund request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Authorization", c.authorizationHeader(timestamp, nonce, signature))

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("post refund: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return nil, fmt.Errorf("read refund response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, parseWechatError(resp.StatusCode, respBody)
	}

	var ok struct {
		RefundID    string `json:"refund_id"`
		OutRefundNo string `json:"out_refund_no"`
		OutTradeNo  string `json:"out_trade_no"`
		Status      string `json:"status"`
	}
	if err := json.Unmarshal(respBody, &ok); err != nil {
		return nil, fmt.Errorf("parse refund response: %w", err)
	}
	if ok.RefundID == "" {
		return nil, errors.New("parse refund response: 缺少 refund_id")
	}
	return &RefundResult{
		RefundID:    ok.RefundID,
		OutRefundNo: ok.OutRefundNo,
		OutTradeNo:  ok.OutTradeNo,
		Status:      ok.Status,
	}, nil
}

// ParseRefundNotify 验签并解密微信退款结果回调。
func (c *Client) ParseRefundNotify(headers NotifyHeaders, body []byte) (*RefundNotifyResult, error) {
	if err := c.verifyNotifySignature(headers, body); err != nil {
		return nil, err
	}
	var envelope notifyEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("%w: 回调报文不是合法 JSON", ErrInvalidNotify)
	}
	if envelope.Resource.Algorithm != "AEAD_AES_256_GCM" {
		return nil, fmt.Errorf("%w: 不支持的加密算法 %q", ErrInvalidNotify, envelope.Resource.Algorithm)
	}
	plaintext, err := decryptNotifyResource(c.apiV3Key, envelope.Resource)
	if err != nil {
		return nil, fmt.Errorf("%w: 解密回调报文失败", ErrInvalidNotify)
	}
	result, err := parseRefundNotifyPayload(plaintext)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidNotify, err)
	}
	if c.mchID != "" && result.MchID != "" && result.MchID != c.mchID {
		return nil, fmt.Errorf("%w: mchid 不匹配", ErrInvalidNotify)
	}
	return result, nil
}

// refundNotifyPayload 是退款回调解密后的 JSON（只取所需字段）。
type refundNotifyPayload struct {
	MchID         string `json:"mchid"`
	OutTradeNo    string `json:"out_trade_no"`
	TransactionID string `json:"transaction_id"`
	OutRefundNo   string `json:"out_refund_no"`
	RefundID      string `json:"refund_id"`
	RefundStatus  string `json:"refund_status"`
	SuccessTime   string `json:"success_time"`
	Amount        struct {
		Total    int64  `json:"total"`
		Refund   int64  `json:"refund"`
		Currency string `json:"currency"`
	} `json:"amount"`
}

// parseRefundNotifyPayload 解析退款回调明文并校验必填字段。
func parseRefundNotifyPayload(raw []byte) (*RefundNotifyResult, error) {
	var payload refundNotifyPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("解析退款报文失败: %w", err)
	}
	if payload.OutRefundNo == "" || payload.RefundStatus == "" {
		return nil, errors.New("退款报文缺少 out_refund_no/refund_status")
	}
	if payload.RefundStatus == RefundStatusSuccess && payload.RefundID == "" {
		return nil, errors.New("成功退款缺少 refund_id")
	}
	if payload.Amount.Currency != "" && payload.Amount.Currency != "CNY" {
		return nil, fmt.Errorf("不支持的币种 %q", payload.Amount.Currency)
	}
	return &RefundNotifyResult{
		MchID:         payload.MchID,
		OutTradeNo:    payload.OutTradeNo,
		TransactionID: payload.TransactionID,
		OutRefundNo:   payload.OutRefundNo,
		RefundID:      payload.RefundID,
		RefundStatus:  payload.RefundStatus,
		SuccessTime:   payload.SuccessTime,
		RefundAmount:  payload.Amount.Refund,
		TotalAmount:   payload.Amount.Total,
	}, nil
}

// Refund 是 mock 通道的退款申请：恒受理并返回 fake 退款单号。
func (m *MockGateway) Refund(_ context.Context, req RefundRequest) (*RefundResult, error) {
	suffix, err := randomHex(8)
	if err != nil {
		return nil, err
	}
	return &RefundResult{
		RefundID:    "mock-refund-" + suffix,
		OutRefundNo: req.OutRefundNo,
		OutTradeNo:  req.OutTradeNo,
		Status:      RefundStatusSuccess,
	}, nil
}

// ParseRefundNotify 是 mock 通道的退款回调解析：直接读取明文 JSON。
func (m *MockGateway) ParseRefundNotify(_ NotifyHeaders, body []byte) (*RefundNotifyResult, error) {
	result, err := parseRefundNotifyPayload(body)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidNotify, err)
	}
	return result, nil
}
