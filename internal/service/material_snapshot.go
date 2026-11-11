package service

import (
	"path"
	"strings"

	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/storage"
)

// newMaterialsResponse 组装资料更新响应（资料完成度 + 当前快照）。
func newMaterialsResponse(reg *model.Registration) *model.UpdateMaterialsResponse {
	return &model.UpdateMaterialsResponse{
		MaterialStatus: MaterialStatusOf(reg),
		Materials:      MaterialsSnapshotOf(reg),
	}
}

// MaterialStatusOf 由报名单字段实时计算资料完成度（API 文档 3.3，T17/T21 复用）。
func MaterialStatusOf(reg *model.Registration) model.MaterialStatus {
	missing := make([]string, 0, model.MaterialTotalFields)
	check := func(field string, present bool) {
		if !present {
			missing = append(missing, field)
		}
	}
	check(model.MaterialFieldPhoto, reg.PhotoPath != nil && *reg.PhotoPath != "")
	check(model.MaterialFieldAudio, reg.AudioPath != nil && *reg.AudioPath != "")
	check(model.MaterialFieldProgramName, nonBlank(reg.ProgramName))
	check(model.MaterialFieldTeacherName, nonBlank(reg.TeacherName))
	check(model.MaterialFieldContactPhone, nonBlank(reg.ContactPhone))
	check(model.MaterialFieldExpressAddress, nonBlank(reg.ExpressAddress))
	check(model.MaterialFieldWorkIntro, nonBlank(reg.WorkIntro))
	check(model.MaterialFieldSpecialReq, nonBlank(reg.SpecialReq))
	return model.MaterialStatus{
		Total:     model.MaterialTotalFields,
		Completed: model.MaterialTotalFields - len(missing),
		Missing:   missing,
	}
}

// MaterialsSnapshotOf 把报名单资料字段转换为对外快照（文件路径映射为访问 URL）。
func MaterialsSnapshotOf(reg *model.Registration) model.MaterialsSnapshot {
	return model.MaterialsSnapshot{
		PhotoURL:       publicURL(reg.PhotoPath),
		AudioURL:       publicURL(reg.AudioPath),
		AudioFilename:  audioFilename(reg.AudioPath),
		ProgramName:    reg.ProgramName,
		TeacherName:    reg.TeacherName,
		ContactPhone:   reg.ContactPhone,
		ExpressAddress: reg.ExpressAddress,
		WorkIntro:      reg.WorkIntro,
		SpecialReq:     reg.SpecialReq,
		UpdatedAt:      reg.MaterialUpdatedAt,
	}
}

func publicURL(relPath *string) *string {
	if relPath == nil || *relPath == "" {
		return nil
	}
	u := storage.PublicURL(*relPath)
	return &u
}

// audioFilename 返回对外展示的音频文件名（数据库只存相对路径，取末段）。
func audioFilename(relPath *string) *string {
	if relPath == nil || *relPath == "" {
		return nil
	}
	name := path.Base(*relPath)
	return &name
}

func nonBlank(s *string) bool {
	return s != nil && strings.TrimSpace(*s) != ""
}
