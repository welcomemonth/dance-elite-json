package service

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
)

// 客服工单字段上限（API 文档 13.3）。
const (
	csTicketUserNameMaxRunes = 50
	csTicketContentMaxRunes  = 1000
	csTicketRemarkMaxRunes   = 500
)

// normalizeCSTicketCreate 校验登记入参并返回规范化字段。
func normalizeCSTicketCreate(req model.CreateCSTicketRequest) (string, *string, string, string, string, error) {
	ticketType := strings.TrimSpace(req.Type)
	if err := validateCSTicketType(ticketType); err != nil {
		return "", nil, "", "", "", err
	}
	phone := strings.TrimSpace(req.Phone)
	if phone == "" {
		return "", nil, "", "", "", httpx.ErrBadRequest("联系电话不能为空")
	}
	content := strings.TrimSpace(req.Content)
	if content == "" {
		return "", nil, "", "", "", httpx.ErrBadRequest("诉求内容不能为空")
	}
	if utf8.RuneCountInString(content) > csTicketContentMaxRunes {
		return "", nil, "", "", "", httpx.ErrBadRequest(fmt.Sprintf("诉求内容不能超过 %d 字", csTicketContentMaxRunes))
	}
	userName := strings.TrimSpace(req.UserName)
	if utf8.RuneCountInString(userName) > csTicketUserNameMaxRunes {
		return "", nil, "", "", "", httpx.ErrBadRequest(fmt.Sprintf("用户标识不能超过 %d 字", csTicketUserNameMaxRunes))
	}
	var userNamePtr *string
	if userName != "" {
		userNamePtr = &userName
	}
	return ticketType, userNamePtr, phone, content, strings.TrimSpace(req.RelatedOrderNo), nil
}

// validateCSTicketType 校验工单类型枚举。
func validateCSTicketType(ticketType string) error {
	switch ticketType {
	case model.CSTicketTypeRefund, model.CSTicketTypeNoIDCard,
		model.CSTicketTypeWaitlist, model.CSTicketTypeOther:
		return nil
	default:
		return httpx.ErrBadRequest("type 取值不合法（refund / no_id_card / waitlist / other）")
	}
}

// validateCSTicketTypeOrEmpty 校验列表类型筛选（空值表示全部）。
func validateCSTicketTypeOrEmpty(ticketType string) error {
	if ticketType == "" {
		return nil
	}
	return validateCSTicketType(ticketType)
}

// validateCSTicketStatusFilter 校验列表状态筛选（空值表示全部）。
func validateCSTicketStatusFilter(status string) error {
	switch status {
	case "", model.CSTicketStatusPending, model.CSTicketStatusProcessing, model.CSTicketStatusDone:
		return nil
	default:
		return httpx.ErrBadRequest("status 取值不合法（pending / processing / done）")
	}
}

// validateCSTicketTransition 校验状态只能前进（pending → processing → done）；
// 同状态视为幂等更新（仅刷新备注）。
func validateCSTicketTransition(from, to string) error {
	if from == to {
		return nil
	}
	allowed := from == model.CSTicketStatusPending && to == model.CSTicketStatusProcessing ||
		from == model.CSTicketStatusProcessing && to == model.CSTicketStatusDone
	if !allowed {
		return httpx.ErrStateDenied(fmt.Sprintf("工单状态不允许从 %s 流转到 %s", from, to))
	}
	return nil
}
