package main

import (
	"fmt"

	config "github.com/meetmonth/dance-elite/internal/configs"
	"github.com/meetmonth/dance-elite/internal/pkg/wxpay"
)

// newPayGateway 按配置装配支付网关：PAY_MOCK=true 走 fake 通道（R2，证书未到
// 位不阻塞联调）；否则构造真实微信支付 V3 客户端（商户私钥解析失败在启动时
// fail-fast，敏感配置只经环境变量注入，见 T3）。
func newPayGateway(cfg *config.Config) (wxpay.Gateway, error) {
	if cfg.PayMock {
		return wxpay.NewMockGateway(), nil
	}
	client, err := wxpay.NewClient(
		cfg.WechatAppID,
		cfg.WechatMchID,
		cfg.WechatMchSerialNo,
		cfg.WechatPrivateKey,
		cfg.WechatNotifyURL,
	)
	if err != nil {
		return nil, fmt.Errorf("初始化微信支付客户端: %w", err)
	}
	return client, nil
}
