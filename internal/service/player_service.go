package service

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/store"
	"github.com/meetmonth/dance-elite/internal/utils"
)

// PlayerService 是选手建档与身份匹配服务（开发计划 T8）。
//
// 职责：身份证合法性校验与字段解析（utils.ParseIDCard）、证件号规范化 +
// HMAC 哈希（utils.HashIDNumber，密钥来自 T3 配置）、按哈希匹配/新建选手档案、
// 按出生年份做组别年龄判定。本服务不暴露 HTTP 接口，由报名下单（T9）、
// 后台人工录入（T16）与成绩导入（T28）在各自事务内调用。
//
// 边界约定：
//   - 证件号明文只存在于请求参数与本函数栈内，不落库、不进日志、不进错误信息；
//   - 出生日期统一解析写入 players.birth_date，组别判定只读该字段，与证件类型解耦；
//   - 在线表单只接受身份证（R7）；护照等其他证件由后台人工录入（T16）。
type PlayerService struct {
	stores  *store.Store
	hmacKey []byte
}

// NewPlayerService 构造选手服务；hmacKey 为证件号哈希密钥（configs.IDCardHMACKey）。
func NewPlayerService(stores *store.Store, hmacKey string) *PlayerService {
	return &PlayerService{stores: stores, hmacKey: []byte(hmacKey)}
}

// IdentityInput 是一次选手身份输入，三种建档入口共用：
// 在线报名（id_card）、后台人工录入（id_card/passport + 手工生日）、成绩导入。
type IdentityInput struct {
	IDType    string // 空串按 id_card 处理（utils.IDTypeIDCard / IDTypePassport）
	IDNumber  string // 明文证件号，仅在本函数栈内使用
	Name      string
	BirthDate string // 仅 passport 必填（YYYY-MM-DD）；id_card 由号码自动解析，本字段忽略
	Gender    string // 仅 passport 可选（model.GenderMale / GenderFemale）；id_card 自动解析
}

// PreparedIdentity 是校验/解析完成、可直接写库的身份字段。
type PreparedIdentity struct {
	IDType     string
	IDCardHash string // 已规范化并 HMAC-SHA256，全库唯一身份键
	Name       string
	BirthDate  *string
	Gender     *string
}

// PrepareIdentity 校验并解析身份输入，返回可写库的规范化身份字段（不写库）。
// 校验失败返回统一错误码：身份证不合法 41001，其余参数错误 40001。
func (s *PlayerService) PrepareIdentity(in IdentityInput) (*PreparedIdentity, error) {
	idType := in.IDType
	if idType == "" {
		idType = utils.IDTypeIDCard
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, httpx.ErrBadRequest("选手姓名不能为空")
	}

	prepared := &PreparedIdentity{
		IDType:     idType,
		IDCardHash: utils.HashIDNumber(s.hmacKey, idType, in.IDNumber),
		Name:       name,
	}

	switch idType {
	case utils.IDTypeIDCard:
		_, birth, gender, err := utils.ParseIDCard(in.IDNumber)
		if err != nil {
			return nil, httpx.ErrInvalidIDNumber("身份证号格式不合法")
		}
		prepared.BirthDate = &birth
		prepared.Gender = &gender
	case utils.IDTypePassport:
		// 护照等其他证件：号码本身只作身份键参与哈希，出生日期必须人工提供（R7）。
		if utils.NormalizeIDNumber(idType, in.IDNumber) == "" {
			return nil, httpx.ErrBadRequest("证件号不能为空")
		}
		birth, err := utils.ParseDate(strings.TrimSpace(in.BirthDate))
		if err != nil {
			return nil, httpx.ErrBadRequest("护照建档必须提供合法出生日期（YYYY-MM-DD）")
		}
		birthStr := birth.Format("2006-01-02")
		prepared.BirthDate = &birthStr
		if g := strings.TrimSpace(in.Gender); g != "" {
			if g != model.GenderMale && g != model.GenderFemale {
				return nil, httpx.ErrBadRequest("性别取值不合法")
			}
			prepared.Gender = &g
		}
	default:
		return nil, httpx.ErrBadRequest("不支持的证件类型")
	}
	return prepared, nil
}

// MatchOrCreate 在调用方事务内匹配或建档选手：按哈希命中返回已有档案，
// 未命中新建（生成 8 位 player_no）。created=true 表示本次新建。
//
// 跨用户同证件号（如家长两个微信号给孩子报名）命中同一 player 档案，
// 重复报名拦截由 T9 的 (player_id, group_id) 部分唯一索引完成。
func (s *PlayerService) MatchOrCreate(ctx context.Context, tx *sql.Tx, in IdentityInput) (*model.Player, bool, error) {
	prepared, err := s.PrepareIdentity(in)
	if err != nil {
		return nil, false, err
	}
	return s.stores.Players.MatchOrCreate(ctx, tx, store.PlayerParams{
		Name:       prepared.Name,
		IDType:     prepared.IDType,
		IDCardHash: prepared.IDCardHash,
		Gender:     prepared.Gender,
		BirthDate:  prepared.BirthDate,
	})
}

// BirthYear 从选手档案的出生日期提取出生年份（组别年龄判定依据）。
// 档案缺失出生日期时返回错误（正常建档路径不会发生，防御未知证件类型等异常数据）。
func BirthYear(p *model.Player) (int, error) {
	if p.BirthDate == nil || *p.BirthDate == "" {
		return 0, fmt.Errorf("player %d has no birth_date", p.ID)
	}
	t, err := utils.ParseDate(*p.BirthDate)
	if err != nil {
		return 0, fmt.Errorf("player %d birth_date %q: %w", p.ID, *p.BirthDate, err)
	}
	return t.Year(), nil
}

// MatchGroupsByBirthYear 筛选出生年份落在 [birth_year_from, birth_year_to]
// 闭区间内的组别（event_groups 口径，含未启用组别——启用与否由调用方另行判断）。
func MatchGroupsByBirthYear(birthYear int, groups []model.EventGroup) []model.EventGroup {
	var out []model.EventGroup
	for _, g := range groups {
		if birthYear >= g.BirthYearFrom && birthYear <= g.BirthYearTo {
			out = append(out, g)
		}
	}
	return out
}

// CheckAgeFit 校验选手出生年份是否落在任一可报组别的年龄区间内；
// 全部不匹配时返回 41002（消息附各组别出生年份区间，供小程序提示）。
// 传入的 groups 应已按启用状态等前置条件过滤。
func CheckAgeFit(birthYear int, groups []model.EventGroup) ([]model.EventGroup, error) {
	matched := MatchGroupsByBirthYear(birthYear, groups)
	if len(matched) > 0 {
		return matched, nil
	}
	return nil, httpx.ErrAgeNotAllowed(fmt.Sprintf("不符合参赛年龄：选手出生于 %d 年，可报组别出生年份要求 %s",
		birthYear, describeGroupYearRanges(groups)))
}

// describeGroupYearRanges 把组别年龄规则汇总成 "U11 2016-2018 年、U13 2014-2015 年"。
func describeGroupYearRanges(groups []model.EventGroup) string {
	parts := make([]string, 0, len(groups))
	for _, g := range groups {
		parts = append(parts, fmt.Sprintf("%s %d-%d 年", g.Level, g.BirthYearFrom, g.BirthYearTo))
	}
	return strings.Join(parts, "、")
}
