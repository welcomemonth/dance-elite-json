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

// MaterialTextInput 是更新资料文本项（API 文档 4.1）的输入。
// nil 表示本次未提交该字段（不修改）；空串表示清空。
type MaterialTextInput struct {
	ProgramName    *string
	TeacherName    *string
	ContactPhone   *string
	ExpressAddress *string
	WorkIntro      *string
	SpecialReq     *string
}

// MaterialService 是参赛资料服务（开发计划 T13）。
//
// 职责：归属与状态/截止校验、文本与文件校验（见 material_validation.go）、
// 文件暂存与单事务落库、证件照压缩头像更新、旧文件清理。
// 文本、证件照、音频由三个独立接口分别更新（API 文档 4.1–4.3）。
type MaterialService struct {
	stores *store.Store
	files  *storage.Store
	now    func() time.Time
}

// NewMaterialService 构造参赛资料服务；files 为 T13 的本地文件存储。
func NewMaterialService(stores *store.Store, files *storage.Store) *MaterialService {
	return &MaterialService{stores: stores, files: files, now: time.Now}
}

// UpdateText 更新 6 项文本资料（PUT /registrations/{id}/materials）。
// 未提交任何文本项时直接返回当前快照，不刷新 material_updated_at。
func (s *MaterialService) UpdateText(ctx context.Context, userID, registrationID int64, in MaterialTextInput) (*model.UpdateMaterialsResponse, error) {
	reg, _, err := s.loadEditable(ctx, userID, registrationID)
	if err != nil {
		return nil, err
	}
	patch, changed, err := s.validateText(ctx, in)
	if err != nil {
		return nil, err
	}
	if !changed {
		return newMaterialsResponse(reg), nil
	}
	params := store.MaterialUpdateParams{
		ProgramName:    patch.ProgramName,
		TeacherName:    patch.TeacherName,
		ContactPhone:   patch.ContactPhone,
		ExpressAddress: patch.ExpressAddress,
		WorkIntro:      patch.WorkIntro,
		SpecialReq:     patch.SpecialReq,
	}
	if err := s.persist(ctx, userID, reg, nil, params, nil, nil); err != nil {
		return nil, err
	}
	return s.snapshot(ctx, reg.ID)
}

// UpdatePhoto 上传/替换证件照（PUT /registrations/{id}/materials/photo），
// 同时生成 512px 头像副本更新 players.avatar_path。
func (s *MaterialService) UpdatePhoto(ctx context.Context, userID, registrationID int64, photo *MaterialFile) (*model.UpdateMaterialsResponse, error) {
	reg, player, err := s.loadEditable(ctx, userID, registrationID)
	if err != nil {
		return nil, err
	}

	var staged []*storage.StagedFile
	defer func() {
		for _, f := range staged {
			s.files.Discard(f)
		}
	}()
	newPhoto, newAvatar, err := s.stagePhoto(reg.EventID, player.PlayerNo, photo, &staged)
	if err != nil {
		return nil, err
	}
	params := store.MaterialUpdateParams{PhotoPath: relPathOf(newPhoto)}
	if err := s.persist(ctx, userID, reg, player, params, newAvatar, staged); err != nil {
		return nil, err
	}

	// 事务提交成功后再清理被替换的旧文件（失败不影响响应，孤儿文件可由
	// 赛后媒体清理兜底）。
	s.removeOld(reg.PhotoPath)
	s.removeOld(player.AvatarPath)
	return s.snapshot(ctx, reg.ID)
}

// UpdateAudio 上传/替换舞蹈音频（PUT /registrations/{id}/materials/audio）。
func (s *MaterialService) UpdateAudio(ctx context.Context, userID, registrationID int64, audio *MaterialFile) (*model.UpdateMaterialsResponse, error) {
	reg, player, err := s.loadEditable(ctx, userID, registrationID)
	if err != nil {
		return nil, err
	}

	var staged []*storage.StagedFile
	defer func() {
		for _, f := range staged {
			s.files.Discard(f)
		}
	}()
	newAudio, err := s.stageAudio(reg.EventID, player.PlayerNo, audio, &staged)
	if err != nil {
		return nil, err
	}
	params := store.MaterialUpdateParams{AudioPath: relPathOf(newAudio)}
	if err := s.persist(ctx, userID, reg, nil, params, nil, staged); err != nil {
		return nil, err
	}
	s.removeOld(reg.AudioPath)
	return s.snapshot(ctx, reg.ID)
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

// stagePhoto 校验并暂存证件照，同时生成头像副本。
func (s *MaterialService) stagePhoto(eventID int64, playerNo string, photo *MaterialFile,
	staged *[]*storage.StagedFile) (newPhoto, newAvatar *storage.StagedFile, err error) {
	if photo == nil {
		return nil, nil, httpx.ErrBadRequest("请选择证件照文件")
	}
	ext, img, err := validatePhoto(photo)
	if err != nil {
		return nil, nil, err
	}
	if _, err := photo.Content.Seek(0, io.SeekStart); err != nil {
		return nil, nil, httpx.ErrInternal(err, "读取证件照失败，请重试")
	}
	newPhoto, err = s.files.StageMaterial(eventID, playerNo, storage.KindPhoto, ext, photo.Content)
	if err != nil {
		return nil, nil, httpx.ErrInternal(err, "保存证件照失败，请稍后重试")
	}
	*staged = append(*staged, newPhoto)

	avatar, err := storage.EncodeAvatar(img, AvatarMaxSide)
	if err != nil {
		return nil, nil, httpx.ErrInternal(err, "生成头像失败，请稍后重试")
	}
	newAvatar, err = s.files.StageAvatar(playerNo, "jpg", bytes.NewReader(avatar))
	if err != nil {
		return nil, nil, httpx.ErrInternal(err, "保存头像失败，请稍后重试")
	}
	*staged = append(*staged, newAvatar)
	return newPhoto, newAvatar, nil
}

// stageAudio 校验并暂存音频。
func (s *MaterialService) stageAudio(eventID int64, playerNo string, audio *MaterialFile,
	staged *[]*storage.StagedFile) (*storage.StagedFile, error) {
	if audio == nil {
		return nil, httpx.ErrBadRequest("请选择音频文件")
	}
	if err := validateAudio(audio); err != nil {
		return nil, err
	}
	if _, err := audio.Content.Seek(0, io.SeekStart); err != nil {
		return nil, httpx.ErrInternal(err, "读取音频失败，请重试")
	}
	newAudio, err := s.files.StageMaterial(eventID, playerNo, storage.KindAudio, "mp3", audio.Content)
	if err != nil {
		return nil, httpx.ErrInternal(err, "保存音频失败，请稍后重试")
	}
	*staged = append(*staged, newAudio)
	return newAudio, nil
}

// persist 在单事务内提交暂存文件并更新资料字段与选手头像。
// 数据库失败时删除已落最终目录的新文件，避免悬空文件。
func (s *MaterialService) persist(ctx context.Context, userID int64, reg *model.Registration, player *model.Player,
	params store.MaterialUpdateParams, newAvatar *storage.StagedFile, staged []*storage.StagedFile) error {
	params.MaterialUpdatedAt = utils.FormatISO(s.now())

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

// snapshot 读取更新后的报名单并组装响应。
func (s *MaterialService) snapshot(ctx context.Context, registrationID int64) (*model.UpdateMaterialsResponse, error) {
	updated, err := s.stores.Registrations.Get(ctx, registrationID)
	if err != nil {
		return nil, httpx.ErrInternal(err, "加载更新后的资料失败")
	}
	return newMaterialsResponse(updated), nil
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
