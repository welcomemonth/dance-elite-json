package service

import (
	"strings"

	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/utils"
)

// validateGroupLevelFilter 校验组别筛选值（空串表示不筛）。
func validateGroupLevelFilter(level string) error {
	switch level {
	case "", model.GroupLevelU11, model.GroupLevelU13, model.GroupLevelU15:
		return nil
	default:
		return httpx.ErrBadRequest("level 取值不合法")
	}
}

// validateDanceTypeFilter 校验舞种筛选值（空串表示不筛）。
func validateDanceTypeFilter(danceType string) error {
	switch danceType {
	case "", model.DanceTypeClassical, model.DanceTypeFolk:
		return nil
	default:
		return httpx.ErrBadRequest("dance_type 取值不合法")
	}
}

// validateAdminOrderStatusFilter 校验订单状态筛选值（空串表示按默认规则）。
func validateAdminOrderStatusFilter(status string) error {
	switch status {
	case "", model.OrderStatusPaid, model.OrderStatusCanceled,
		model.OrderStatusRefunding, model.OrderStatusRefunded:
		return nil
	default:
		return httpx.ErrBadRequest("status 取值不合法（paid / canceled / refunding / refunded）")
	}
}

// validateAdminOrderSourceFilter 校验订单来源筛选值（空串表示不筛）。
func validateAdminOrderSourceFilter(source string) error {
	switch source {
	case "", model.OrderSourceMiniapp, model.OrderSourceManual:
		return nil
	default:
		return httpx.ErrBadRequest("source 取值不合法（miniapp / manual）")
	}
}

// validateAdminRegistrationStatusFilter 校验报名状态筛选值（空串默认 active）。
func validateAdminRegistrationStatusFilter(status string) error {
	switch status {
	case "", model.RegistrationStatusPendingPayment, model.RegistrationStatusActive,
		model.RegistrationStatusCanceled, model.RegistrationStatusWithdrawn:
		return nil
	default:
		return httpx.ErrBadRequest("status 取值不合法（active / canceled / withdrawn / pending_payment）")
	}
}

// validateMaterialStateFilter 校验资料完成度筛选值。
func validateMaterialStateFilter(state string) error {
	switch state {
	case "", "complete", "incomplete":
		return nil
	default:
		return httpx.ErrBadRequest("material_state 取值不合法（complete / incomplete）")
	}
}

// validMissingItem 判断缺失资料项名是否受支持。
func validMissingItem(item string) bool {
	switch item {
	case model.MaterialFieldPhoto, model.MaterialFieldAudio,
		model.MaterialFieldProgramName, model.MaterialFieldContactPhone:
		return true
	default:
		return false
	}
}

// validateMissingItemFilter 校验缺失资料项筛选值（空串表示不筛）。
func validateMissingItemFilter(item string) error {
	if item == "" || validMissingItem(item) {
		return nil
	}
	return httpx.ErrBadRequest("missing_item 取值不合法（photo / audio / program_name / contact_phone）")
}

// normalizeAdminPaidRange 归一化支付时间范围筛选：接受 ISO8601 或 YYYY-MM-DD，
// 日期形式按当天起止补全；返回全库统一时间格式（可直接字符串比较）。
func normalizeAdminPaidRange(from, to string) (string, string, error) {
	normalizedFrom, err := normalizeAdminTime(from, false)
	if err != nil {
		return "", "", err
	}
	normalizedTo, err := normalizeAdminTime(to, true)
	if err != nil {
		return "", "", err
	}
	if normalizedFrom != "" && normalizedTo != "" && normalizedFrom > normalizedTo {
		return "", "", httpx.ErrBadRequest("paid_from 不能晚于 paid_to")
	}
	return normalizedFrom, normalizedTo, nil
}

// normalizeAdminTime 归一化单个时间筛选值；endOfDay 为 true 时日期形式补到 23:59:59。
func normalizeAdminTime(value string, endOfDay bool) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if t, err := utils.ParseISO(value); err == nil {
		return utils.FormatISO(t), nil
	}
	if d, err := utils.ParseDate(value); err == nil {
		date := d.Format("2006-01-02")
		if endOfDay {
			return date + "T23:59:59+08:00", nil
		}
		return date + "T00:00:00+08:00", nil
	}
	return "", httpx.ErrBadRequest("时间筛选格式不合法，应为 ISO8601 或 YYYY-MM-DD")
}
