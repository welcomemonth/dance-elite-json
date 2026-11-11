package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/store"
)

// 客服工单审计动作（admin_logs.action）。
const (
	adminActionCSTicketCreate = "admin.cs_ticket.create"
	adminActionCSTicketUpdate = "admin.cs_ticket.update"
)

// CSTicketService 是客服工单服务（开发计划 T16，API 文档 13.3）：
// 客服登记退款/无证件号报名/候补咨询等诉求，管理员处理流转并留痕。
// 工单关联退款工单或人工报名由各自流程回填关联字段。
type CSTicketService struct {
	stores *store.Store
}

// NewCSTicketService 构造客服工单服务。
func NewCSTicketService(stores *store.Store) *CSTicketService {
	return &CSTicketService{stores: stores}
}

// Create 登记客服工单（cs 与 admin 均可）：type/phone/content 必填，
// related_order_no 可选，存在时必须能解析为订单（40401）。
func (s *CSTicketService) Create(ctx context.Context, adminID int64, req model.CreateCSTicketRequest) (*model.CreateCSTicketResponse, error) {
	ticketType, userName, phone, content, orderNo, err := normalizeCSTicketCreate(req)
	if err != nil {
		return nil, err
	}

	var ticketID int64
	err = s.stores.WithTx(ctx, func(tx *sql.Tx) error {
		var relatedOrderID *int64
		if orderNo != "" {
			order, err := s.stores.Orders.GetByOrderNoTx(ctx, tx, orderNo)
			if errors.Is(err, store.ErrNotFound) {
				return httpx.ErrNotFound("关联订单不存在")
			}
			if err != nil {
				return httpx.ErrInternal(err, "加载订单失败，请稍后重试")
			}
			relatedOrderID = &order.ID
		}
		id, err := s.stores.CSTickets.CreateTx(ctx, tx, store.CSTicketParams{
			Type:           ticketType,
			UserName:       userName,
			Phone:          phone,
			Content:        content,
			RelatedOrderID: relatedOrderID,
			CreatedBy:      adminID,
		})
		if err != nil {
			return httpx.ErrInternal(err, "登记客服工单失败，请稍后重试")
		}
		if err := adminLog(ctx, s.stores, tx, &adminID, adminActionCSTicketCreate,
			strPtr("cs_ticket"), &id, map[string]any{
				"type":             ticketType,
				"related_order_no": orderNo,
			}); err != nil {
			return httpx.ErrInternal(err, "记录操作日志失败，请稍后重试")
		}
		ticketID = id
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &model.CreateCSTicketResponse{TicketID: ticketID, Status: model.CSTicketStatusPending}, nil
}

// List 返回后台工单分页列表：cs 只能看到自己登记的工单，admin 看全量。
func (s *CSTicketService) List(ctx context.Context, adminID int64, role string, q model.AdminCSTicketListQuery) (*model.AdminCSTicketListResponse, error) {
	if err := validateCSTicketTypeOrEmpty(q.Type); err != nil {
		return nil, err
	}
	if err := validateCSTicketStatusFilter(q.Status); err != nil {
		return nil, err
	}
	page, pageSize := normalizePage(q.Page, q.PageSize)
	filter := store.AdminCSTicketFilter{
		Type:     q.Type,
		Status:   q.Status,
		Page:     page,
		PageSize: pageSize,
	}
	if role == model.RoleCS {
		filter.CreatedBy = adminID
	}
	rows, total, err := s.stores.CSTickets.ListAdmin(ctx, filter)
	if err != nil {
		return nil, httpx.ErrInternal(err, "加载客服工单失败，请稍后重试")
	}

	list := make([]model.AdminCSTicketItem, 0, len(rows))
	for _, r := range rows {
		list = append(list, model.AdminCSTicketItem{
			TicketID:              r.TicketID,
			Type:                  r.Type,
			UserName:              r.UserName,
			Phone:                 r.Phone,
			Content:               r.Content,
			RelatedOrderNo:        r.RelatedOrderNo,
			RelatedRegistrationID: r.RelatedRegistrationID,
			Status:                r.Status,
			Remark:                r.Remark,
			CreatedBy:             r.CreatedBy,
			CreatedByName:         r.CreatedByName,
			CreatedAt:             r.CreatedAt,
			UpdatedAt:             r.UpdatedAt,
		})
	}
	return &model.AdminCSTicketListResponse{
		List: list, Total: total, Page: page, PageSize: pageSize,
	}, nil
}

// Update 流转工单（仅 admin / super_admin，路由层控制）：pending → processing → done，
// 同状态视为幂等更新备注；每次流转写 admin_logs。
func (s *CSTicketService) Update(ctx context.Context, adminID, ticketID int64, req model.UpdateCSTicketRequest) (*model.UpdateCSTicketResponse, error) {
	target := strings.TrimSpace(req.Status)
	switch target {
	case model.CSTicketStatusPending, model.CSTicketStatusProcessing, model.CSTicketStatusDone:
	default:
		return nil, httpx.ErrBadRequest("status 取值不合法（pending / processing / done）")
	}
	remark := strings.TrimSpace(req.Remark)
	if utf8.RuneCountInString(remark) > csTicketRemarkMaxRunes {
		return nil, httpx.ErrBadRequest(fmt.Sprintf("跟进备注不能超过 %d 字", csTicketRemarkMaxRunes))
	}

	err := s.stores.WithTx(ctx, func(tx *sql.Tx) error {
		ticket, err := s.stores.CSTickets.GetTx(ctx, tx, ticketID)
		if errors.Is(err, store.ErrNotFound) {
			return httpx.ErrNotFound("客服工单不存在")
		}
		if err != nil {
			return httpx.ErrInternal(err, "加载客服工单失败，请稍后重试")
		}
		if err := validateCSTicketTransition(ticket.Status, target); err != nil {
			return err
		}

		var remarkPtr *string
		if remark != "" {
			remarkPtr = &remark
		}
		if err := s.stores.CSTickets.UpdateStatusTx(ctx, tx, ticketID, ticket.Status, target, remarkPtr); err != nil {
			if errors.Is(err, store.ErrConflict) {
				return httpx.ErrStateDenied("工单状态已变化，请刷新后重试")
			}
			return httpx.ErrInternal(err, "更新工单状态失败，请稍后重试")
		}
		if err := adminLog(ctx, s.stores, tx, &adminID, adminActionCSTicketUpdate,
			strPtr("cs_ticket"), &ticketID, map[string]any{
				"from_status": ticket.Status,
				"to_status":   target,
				"remark":      remark,
			}); err != nil {
			return httpx.ErrInternal(err, "记录操作日志失败，请稍后重试")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &model.UpdateCSTicketResponse{TicketID: ticketID, Status: target}, nil
}
