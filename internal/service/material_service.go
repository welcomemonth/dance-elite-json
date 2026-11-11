package service

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"time"

	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/pkg/storage"
	"github.com/meetmonth/dance-elite/internal/store"
	"github.com/meetmonth/dance-elite/internal/utils"
)

// MaterialFile 是一次 multipart 上传的文件内容。Filename 仅用于扩展名校验，
// 落盘文件名由服务端随机生成。
type MaterialFile struct {
	Filename string
	Size     int64
	Content  io.ReadSeekCloser
}

// MaterialUpdateInput 是更新参赛资料（API 文档 4.1）的输入。
// nil 表示本次未提交该字段（不修改）；文本字段传空串表示清空。
type MaterialUpdateInput struct {
	Photo          *MaterialFile
	Audio          *MaterialFile
	ProgramName    *string
	TeacherName    *string
	ContactPhone   *string
	ExpressAddress *string
	WorkIntro      *string
	SpecialReq     *string
}

// MaterialService 是参赛资料服务（开发计划 T13，API 文档 4.1）。
//
// 职责：归属与状态/截止校验、文本与文件校验（见 material_validation.go）、
// 文件暂存与单事务落库、证件照压缩头像更新、旧文件清理。
type MaterialService struct {
	stores *store.Store
	files  *storage.Store
	now    func() time.Time
}

// NewMaterialService 构造参赛资料服务；files 为 T13 的本地文件存储。
func NewMaterialService(stores *store.Store, files *storage.Store) *MaterialService {
	return &MaterialService{stores: stores, files: files, now: time.Now}
}

// Update 更新报名单资料（PUT /registrations/{id}/materials）。
//
// 校验顺序：归属（40401）→ 报名状态（41009）→ 资料截止（41008）→
// 文本字段（40001）→ 证件照/音频格式大小时长（40001）→ 落临时文件。
// 全部通过后单事务提交文件与资料字段，提交后清理被替换的旧文件。
func (s *MaterialService) Update(ctx context.Context, userID, registrationID int64, in MaterialUpdateInput) (*model.UpdateMaterialsResponse, error) {
	reg, player, err := s.loadEditable(ctx, userID, registrationID)
	if err != nil {
		return nil, err
	}

	textPatch, textChanged, err := s.validateText(ctx, in)
	if err != nil {
		return nil, err
	}

	// 落临时文件（校验通过后才进入最终目录，失败不影响已有资料）。
	var staged []*storage.StagedFile
	defer func() {
		for _, f := range staged {
			s.files.Discard(f)
		}
	}()
	newPhoto, newAudio, newAvatar, err := s.stageFiles(reg.EventID, player.PlayerNo, in, &staged)
	if err != nil {
		return nil, err
	}

	// 无任何变更：直接返回当前快照（不刷新 material_updated_at）。
	if len(staged) == 0 && !textChanged {
		return newMaterialsResponse(reg), nil
	}

	if err := s.persist(ctx, userID, reg, player, textPatch, newPhoto, newAudio, newAvatar, staged); err != nil {
		return nil, err
	}

	// 事务提交成功后再清理被替换的旧文件（失败不影响响应，孤儿文件可由
	// 赛后媒体清理兜底）。
	if newPhoto != nil {
		s.removeOld(reg.PhotoPath)
	}
	if newAudio != nil {
		s.removeOld(reg.AudioPath)
	}
	if newAvatar != nil {
		s.removeOld(player.AvatarPath)
	}

	updated, err := s.stores.Registrations.Get(ctx, reg.ID)
	if err != nil {
		return nil, httpx.ErrInternal(err, "加载更新后的资料失败")
	}
	return newMaterialsResponse(updated), nil
}

// loadEditable 加载报名单/选手并完成归属、状态与资料截止校验。
func (s *MaterialService) loadEditable(ctx context.Context, userID, registrationID int64) (*model.Registration, *model.Player, error) {
	reg, err := s.stores.Registrations.Get(ctx, registrationID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil, httpx.ErrNotFound("报名不存在或非本人")
	}
	if err != nil {
		return nil, nil, httpx.ErrInternal(err, "加载报名信息失败，请稍后重试")
	}
	// 不区分"不存在"与"非本人"，避免枚举他人报名单。
	if reg.UserID != userID {
		return nil, nil, httpx.ErrNotFound("报名不存在或非本人")
	}
	if reg.Status != model.RegistrationStatusActive {
		return nil, nil, httpx.ErrMaterialNotAllowed(materialStatusMessage(reg.Status))
	}

	event, err := s.stores.Events.Get(ctx, reg.EventID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil, httpx.ErrInternal(err, "报名关联的赛事不存在")
	}
	if err != nil {
		return nil, nil, httpx.ErrInternal(err, "加载赛事信息失败，请稍后重试")
	}
	if event.MaterialCheckAt != nil && *event.MaterialCheckAt != "" {
		deadline, err := utils.ParseISO(*event.MaterialCheckAt)
		if err != nil {
			return nil, nil, httpx.ErrInternal(err, "赛事资料截止时间配置异常")
		}
		if !s.now().Before(deadline) {
			return nil, nil, httpx.ErrMaterialDeadline("已过资料上传截止时间，资料不可修改")
		}
	}

	player, err := s.stores.Players.GetByID(ctx, reg.PlayerID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil, httpx.ErrInternal(err, "报名关联的选手档案不存在")
	}
	if err != nil {
		return nil, nil, httpx.ErrInternal(err, "加载选手档案失败，请稍后重试")
	}
	return reg, player, nil
}

// stageFiles 校验并暂存本次上传的证件照/音频，同时为证件照生成头像副本。
func (s *MaterialService) stageFiles(eventID int64, playerNo string, in MaterialUpdateInput, staged *[]*storage.StagedFile) (
	newPhoto, newAudio, newAvatar *storage.StagedFile, err error) {
	if in.Photo != nil {
		ext, img, err := validatePhoto(in.Photo)
		if err != nil {
			return nil, nil, nil, err
		}
		if _, err := in.Photo.Content.Seek(0, io.SeekStart); err != nil {
			return nil, nil, nil, httpx.ErrInternal(err, "读取证件照失败，请重试")
		}
		newPhoto, err = s.files.StageMaterial(eventID, playerNo, storage.KindPhoto, ext, in.Photo.Content)
		if err != nil {
			return nil, nil, nil, httpx.ErrInternal(err, "保存证件照失败，请稍后重试")
		}
		*staged = append(*staged, newPhoto)

		avatar, err := storage.EncodeAvatar(img, AvatarMaxSide)
		if err != nil {
			return nil, nil, nil, httpx.ErrInternal(err, "生成头像失败，请稍后重试")
		}
		newAvatar, err = s.files.StageAvatar(playerNo, "jpg", bytes.NewReader(avatar))
		if err != nil {
			return nil, nil, nil, httpx.ErrInternal(err, "保存头像失败，请稍后重试")
		}
		*staged = append(*staged, newAvatar)
	}

	if in.Audio != nil {
		if err := validateAudio(in.Audio); err != nil {
			return nil, nil, nil, err
		}
		if _, err := in.Audio.Content.Seek(0, io.SeekStart); err != nil {
			return nil, nil, nil, httpx.ErrInternal(err, "读取音频失败，请重试")
		}
		newAudio, err = s.files.StageMaterial(eventID, playerNo, storage.KindAudio, "mp3", in.Audio.Content)
		if err != nil {
			return nil, nil, nil, httpx.ErrInternal(err, "保存音频失败，请稍后重试")
		}
		*staged = append(*staged, newAudio)
	}
	return newPhoto, newAudio, newAvatar, nil
}

// persist 在单事务内提交暂存文件并更新资料字段与选手头像。
// 数据库失败时删除已落最终目录的新文件，避免悬空文件。
func (s *MaterialService) persist(ctx context.Context, userID int64, reg *model.Registration, player *model.Player,
	textPatch materialTextPatch, newPhoto, newAudio, newAvatar *storage.StagedFile, staged []*storage.StagedFile) error {
	params := store.MaterialUpdateParams{
		PhotoPath:         relPathOf(newPhoto),
		AudioPath:         relPathOf(newAudio),
		ProgramName:       textPatch.ProgramName,
		TeacherName:       textPatch.TeacherName,
		ContactPhone:      textPatch.ContactPhone,
		ExpressAddress:    textPatch.ExpressAddress,
		WorkIntro:         textPatch.WorkIntro,
		SpecialReq:        textPatch.SpecialReq,
		MaterialUpdatedAt: utils.FormatISO(s.now()),
	}

	var committed []*storage.StagedFile
	err := s.stores.WithTx(ctx, func(tx *sql.Tx) error {
		for _, f := range staged {
			if err := s.files.Commit(f); err != nil {
				return err
			}
			committed = append(committed, f)
		}
		if err := s.stores.Registrations.UpdateMaterialsTx(ctx, tx, reg.ID, userID, params); err != nil {
			return err
		}
		if newAvatar != nil {
			if err := s.stores.Players.UpdateAvatarTx(ctx, tx, player.ID, newAvatar.RelPath); err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil {
		return nil
	}
	for _, f := range committed {
		_ = s.files.Remove(f.RelPath)
	}
	if errors.Is(err, store.ErrConflict) {
		return httpx.ErrMaterialNotAllowed("报名状态已变化，资料不可修改")
	}
	if errors.Is(err, store.ErrNotFound) {
		return httpx.ErrInternal(err, "报名或选手档案不存在")
	}
	return httpx.ErrInternal(err, "保存资料失败，请稍后重试")
}

// relPathOf 返回已暂存文件的新相对路径（未上传返回 nil）。
func relPathOf(f *storage.StagedFile) *string {
	if f == nil {
		return nil
	}
	p := f.RelPath
	return &p
}

// removeOld 尽力删除被替换的旧文件；失败不影响业务结果。
func (s *MaterialService) removeOld(relPath *string) {
	if relPath == nil || *relPath == "" {
		return
	}
	_ = s.files.Remove(*relPath)
}

// materialStatusMessage 返回非 active 报名不可修改资料的用户可见原因。
func materialStatusMessage(status string) string {
	switch status {
	case model.RegistrationStatusPendingPayment:
		return "报名尚未完成支付，资料不可修改"
	case model.RegistrationStatusCanceled:
		return "报名已取消，资料不可修改"
	case model.RegistrationStatusWithdrawn:
		return "报名已退赛，资料不可修改"
	default:
		return "当前报名状态不允许修改资料"
	}
}
