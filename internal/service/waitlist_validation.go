package service

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/utils"
)

// preparedWaitlist 是校验并规范化后的候补提交字段。
// 证件号明文不保留：只留下 HMAC 哈希、脱敏值与出生年份。
type preparedWaitlist struct {
	Name         string
	Phone        string
	Institution  *string
	IDCardHash   *string
	IDCardMasked *string
	BirthYear    *int
}

// prepareWaitlistInput 校验候补表单（API 文档 5.1）。
// 身份证号可选：提供时按身份证规则校验（41001）并哈希；不提供时只登记姓名与电话（R7）。
func (s *WaitlistService) prepareWaitlistInput(req model.SubmitWaitlistRequest) (*preparedWaitlist, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, httpx.ErrBadRequest("选手姓名不能为空")
	}
	if utf8.RuneCountInString(name) > 30 {
		return nil, httpx.ErrBadRequest("选手姓名不能超过 30 字")
	}

	phone := strings.TrimSpace(req.Phone)
	if !phonePattern.MatchString(phone) {
		return nil, httpx.ErrBadRequest("联系电话格式不正确（11 位手机号）")
	}

	out := &preparedWaitlist{Name: name, Phone: phone}
	if inst := strings.TrimSpace(req.Institution); inst != "" {
		if utf8.RuneCountInString(inst) > 50 {
			return nil, httpx.ErrBadRequest("机构名称不能超过 50 字")
		}
		out.Institution = &inst
	}

	idNumber := strings.TrimSpace(req.IDCardNo)
	if idNumber == "" {
		return out, nil
	}
	prepared, err := s.players.PrepareIdentity(IdentityInput{
		IDType:   utils.IDTypeIDCard,
		IDNumber: idNumber,
		Name:     name,
	})
	if err != nil {
		return nil, err
	}
	hash := prepared.IDCardHash
	masked := utils.MaskIDNumber(idNumber)
	out.IDCardHash = &hash
	out.IDCardMasked = &masked
	if prepared.BirthDate != nil {
		birth, err := utils.ParseDate(*prepared.BirthDate)
		if err != nil {
			return nil, httpx.ErrInternal(err, "解析出生日期失败")
		}
		year := birth.Year()
		out.BirthYear = &year
	}
	return out, nil
}

// checkWaitlistGroup 校验候补报名单元：存在且属于该赛事、年龄匹配、启用、名额已满。
// 名额未满时引导用户直接报名（API 文档 5.1 失败响应 40001）。
func checkWaitlistGroup(eventID int64, g *model.EventGroup, birthYear *int) error {
	if g.EventID != eventID {
		return httpx.ErrNotFound("赛事组别不存在")
	}
	if birthYear != nil && (*birthYear < g.BirthYearFrom || *birthYear > g.BirthYearTo) {
		return httpx.ErrAgeNotAllowed(fmt.Sprintf(
			"不符合参赛年龄：选手出生于 %d 年，%s 组别要求出生年份 %d-%d 年",
			*birthYear, g.Level, g.BirthYearFrom, g.BirthYearTo))
	}
	if !g.Enabled {
		return httpx.ErrGroupDisabled(fmt.Sprintf("该组别（%s %s）已关闭报名", g.Level, danceTypeLabel(g.DanceType)))
	}
	if g.Taken < g.Quota {
		return httpx.ErrBadRequest(fmt.Sprintf("该组别（%s %s）名额未满，请直接报名", g.Level, danceTypeLabel(g.DanceType)))
	}
	return nil
}

// validateWaitlistStatusFilter 校验后台列表的 status 筛选值。
func validateWaitlistStatusFilter(status string) error {
	if status == "" {
		return nil
	}
	switch status {
	case model.WaitlistStatusWaiting, model.WaitlistStatusNotified,
		model.WaitlistStatusConverted, model.WaitlistStatusGivenUp, model.WaitlistStatusRemoved:
		return nil
	}
	return httpx.ErrBadRequest("status 取值不合法")
}

// validateWaitlistNote 校验后台通知结果枚举（API 文档 12.2 notify.note）。
func validateWaitlistNote(note string) error {
	switch note {
	case model.WaitlistNoteConfirmedIntent, model.WaitlistNoteNoAnswer, model.WaitlistNoteCallbackLater:
		return nil
	}
	return httpx.ErrBadRequest("note 取值不合法（confirmed_intent / no_answer / callback_later）")
}

// isWaitlistActive 返回候补是否仍在队列中（waiting/notified）。
func isWaitlistActive(status string) bool {
	return status == model.WaitlistStatusWaiting || status == model.WaitlistStatusNotified
}
