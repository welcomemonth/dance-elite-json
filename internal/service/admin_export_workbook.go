package service

import (
	"fmt"
	"strings"

	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/store"
	"github.com/xuri/excelize/v2"
)

// exportSheetName 是导出名单的工作表名。
const exportSheetName = "报名名单"

// buildRegistrationWorkbook 生成报名名单 Excel（按赛事导出，签到/对账用）。
//
// 列固定为：订单号、姓名、选手编号、组别、舞种、机构、资料完成度、缺失资料项、
// 联系电话、报名状态、报名时间。**不得加入证件号字段**（系统只存 HMAC 哈希，
// 导出不含任何形态的证件号）。
func buildRegistrationWorkbook(rows []store.AdminRegistrationRow) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	if err := f.SetSheetName("Sheet1", exportSheetName); err != nil {
		return nil, fmt.Errorf("rename export sheet: %w", err)
	}

	headers := []any{"订单号", "选手姓名", "选手编号", "组别", "舞种", "机构",
		"资料完成度", "缺失资料项", "联系电话", "报名状态", "报名时间"}
	if err := f.SetSheetRow(exportSheetName, "A1", &headers); err != nil {
		return nil, fmt.Errorf("write export header: %w", err)
	}
	for i, row := range rows {
		status := MaterialStatusOf(&row.Registration)
		values := []any{
			ptrValue(row.OrderNo, ""),
			row.PlayerName,
			row.PlayerNo,
			row.Level,
			danceTypeLabel(row.Registration.DanceType),
			ptrValue(row.Registration.Institution, ""),
			fmt.Sprintf("%d/%d", status.Completed, status.Total),
			missingItemsLabel(status.Missing),
			ptrValue(row.ContactPhone, ""),
			registrationStatusLabel(row.Registration.Status),
			row.Registration.CreatedAt,
		}
		cell := fmt.Sprintf("A%d", i+2)
		if err := f.SetSheetRow(exportSheetName, cell, &values); err != nil {
			return nil, fmt.Errorf("write export row %d: %w", i+1, err)
		}
	}

	if err := styleExportSheet(f); err != nil {
		return nil, err
	}
	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("write export workbook: %w", err)
	}
	return buf.Bytes(), nil
}

// styleExportSheet 设置表头样式、列宽与首行冻结。
func styleExportSheet(f *excelize.File) error {
	style, err := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true},
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"DDEBF7"}},
	})
	if err != nil {
		return fmt.Errorf("create export style: %w", err)
	}
	if err := f.SetCellStyle(exportSheetName, "A1", "K1", style); err != nil {
		return fmt.Errorf("apply export header style: %w", err)
	}
	widths := map[string]float64{
		"A": 24, "B": 12, "C": 12, "D": 8, "E": 14, "F": 24,
		"G": 12, "H": 24, "I": 16, "J": 10, "K": 24,
	}
	for column, width := range widths {
		if err := f.SetColWidth(exportSheetName, column, column, width); err != nil {
			return fmt.Errorf("set export column %s width: %w", column, err)
		}
	}
	if err := f.SetPanes(exportSheetName, &excelize.Panes{
		Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft",
	}); err != nil {
		return fmt.Errorf("freeze export header: %w", err)
	}
	return nil
}

// registrationStatusLabel 把报名状态枚举转换为中文展示。
func registrationStatusLabel(status string) string {
	switch status {
	case model.RegistrationStatusActive:
		return "有效"
	case model.RegistrationStatusCanceled:
		return "已取消"
	case model.RegistrationStatusWithdrawn:
		return "已退赛"
	case model.RegistrationStatusPendingPayment:
		return "待支付"
	default:
		return status
	}
}

// missingItemsLabel 把缺失资料项字段名转换为中文并连接。
func missingItemsLabel(items []string) string {
	if len(items) == 0 {
		return ""
	}
	labels := make([]string, 0, len(items))
	for _, item := range items {
		labels = append(labels, materialFieldLabel(item))
	}
	return strings.Join(labels, "、")
}

// materialFieldLabel 把资料项字段名转换为中文。
func materialFieldLabel(field string) string {
	switch field {
	case model.MaterialFieldPhoto:
		return "证件照"
	case model.MaterialFieldAudio:
		return "舞蹈音频"
	case model.MaterialFieldProgramName:
		return "节目名称"
	case model.MaterialFieldTeacherName:
		return "指导教师"
	case model.MaterialFieldContactPhone:
		return "联系电话"
	case model.MaterialFieldExpressAddress:
		return "收件地址"
	case model.MaterialFieldWorkIntro:
		return "作品简介"
	case model.MaterialFieldSpecialReq:
		return "特殊要求"
	default:
		return field
	}
}
