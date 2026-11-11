package service

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/utils"
)

// 人工报名字段上限。
const (
	manualPlayerNameMaxRunes  = 50
	manualInstitutionMaxRunes = 50
	manualNationalityMaxRunes = 50
	manualRemarkMaxRunes      = 500
)

// manualRegistrationInput 是校验/规范化后的人工报名入参（证件号明文仅在栈内使用）。
type manualRegistrationInput struct {
	EventID      int64
	GroupID      int64
	PlayerName   string
	IDType       string
	IDNumber     string
	BirthDate    string
	Gender       string
	Nationality  *string
	Institution  *string
	ContactPhone string
	ChargeType   string
	Amount       int64
	CSTicketID   int64
	Remark       *string
}

// normalizeManualRegistration 校验人工报名入参并返回规范化字段。
// id_type 缺省时按证件号格式自动判定（合法大陆身份证 → id_card）。
func normalizeManualRegistration(req model.ManualRegistrationRequest) (*manualRegistrationInput, error) {
	if req.EventID <= 0 || req.GroupID <= 0 {
		return nil, httpx.ErrBadRequest("event_id / group_id 不合法")
	}
	name := strings.TrimSpace(req.PlayerName)
	if name == "" {
		return nil, httpx.ErrBadRequest("选手姓名不能为空")
	}
	if utf8.RuneCountInString(name) > manualPlayerNameMaxRunes {
		return nil, httpx.ErrBadRequest(fmt.Sprintf("选手姓名不能超过 %d 字", manualPlayerNameMaxRunes))
	}
	idNumber := strings.TrimSpace(req.IDCardNo)
	if idNumber == "" {
		return nil, httpx.ErrBadRequest("证件号不能为空")
	}
	idType := strings.TrimSpace(req.IDType)
	if idType == "" {
		idType = detectIDType(idNumber)
		if idType == "" {
			return nil, httpx.ErrBadRequest("无法识别证件类型，请选择护照 / 港澳台证件 / 其他")
		}
	}
	switch idType {
	case utils.IDTypeIDCard, utils.IDTypePassport, utils.IDTypeHKMacau, utils.IDTypeOther:
	default:
		return nil, httpx.ErrBadRequest("id_type 取值不合法（id_card / passport / hk_macau / other）")
	}
	phone := strings.TrimSpace(req.ContactPhone)
	if phone == "" {
		return nil, httpx.ErrBadRequest("联系电话不能为空")
	}

	chargeType := strings.TrimSpace(req.ChargeType)
	amount := req.Amount
	switch chargeType {
	case model.ManualChargeFree:
		amount = 0
	case model.ManualChargeOfflinePaid:
		if amount <= 0 {
			return nil, httpx.ErrBadRequest("线下收款金额必须大于 0")
		}
	default:
		return nil, httpx.ErrBadRequest("charge_type 取值不合法（offline_paid / free）")
	}

	in := &manualRegistrationInput{
		EventID:      req.EventID,
		GroupID:      req.GroupID,
		PlayerName:   name,
		IDType:       idType,
		IDNumber:     idNumber,
		BirthDate:    strings.TrimSpace(req.BirthDate),
		Gender:       strings.TrimSpace(req.Gender),
		ContactPhone: phone,
		ChargeType:   chargeType,
		Amount:       amount,
		CSTicketID:   req.CSTicketID,
	}
	if in.CSTicketID < 0 {
		return nil, httpx.ErrBadRequest("cs_ticket_id 不合法")
	}
	if v := strings.TrimSpace(req.Nationality); v != "" {
		if utf8.RuneCountInString(v) > manualNationalityMaxRunes {
			return nil, httpx.ErrBadRequest(fmt.Sprintf("国籍/地区不能超过 %d 字", manualNationalityMaxRunes))
		}
		in.Nationality = &v
	}
	if v := strings.TrimSpace(req.Institution); v != "" {
		if utf8.RuneCountInString(v) > manualInstitutionMaxRunes {
			return nil, httpx.ErrBadRequest(fmt.Sprintf("机构名称不能超过 %d 字", manualInstitutionMaxRunes))
		}
		in.Institution = &v
	}
	if v := strings.TrimSpace(req.Remark); v != "" {
		if utf8.RuneCountInString(v) > manualRemarkMaxRunes {
			return nil, httpx.ErrBadRequest(fmt.Sprintf("备注不能超过 %d 字", manualRemarkMaxRunes))
		}
		in.Remark = &v
	}
	return in, nil
}

// detectIDType 按证件号格式自动判定类型：合法大陆身份证 → id_card，否则返回空串
// （要求调用方显式选择特殊证件类型）。
func detectIDType(idNumber string) string {
	if _, _, _, err := utils.ParseIDCard(idNumber); err == nil {
		return utils.IDTypeIDCard
	}
	return ""
}
