package model

// WxPayParams 是微信小程序支付参数（API 文档 3.1 pay_params）。
// 字段名遵循微信 wx.requestPayment 协议约定（camelCase），是本项目
// snake_case 命名规则的文档级例外。
type WxPayParams struct {
	TimeStamp string `json:"timeStamp"`
	NonceStr  string `json:"nonceStr"`
	Package   string `json:"package"`
	SignType  string `json:"signType"`
	PaySign   string `json:"paySign"`
}

// PrepayResponse 是 POST /api/v1/orders/{order_id}/prepay 的响应
// （API 文档 3.1b，T10）。
type PrepayResponse struct {
	OrderNo   string      `json:"order_no"`
	PayParams WxPayParams `json:"pay_params"`
}
