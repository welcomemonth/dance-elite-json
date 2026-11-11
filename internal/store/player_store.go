package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"

	"github.com/meetmonth/dance-elite/internal/model"
)

// PlayerStore 是选手档案（players 表）的数据访问层。
type PlayerStore struct{ db *DB }

// NewPlayerStore 构造选手 store。
func NewPlayerStore(db *DB) *PlayerStore { return &PlayerStore{db: db} }

const playerColumns = `id, player_no, name, id_card_hash, id_type, gender, birth_date,
	nationality, avatar_path, merged_into, created_at, updated_at`

// playerNoAlphabet 是 player_no 的字符集：8 位大小写字母+数字（数据库表结构设计决策 #1）。
const playerNoAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

// maxPlayerNoRetries 是 player_no 撞唯一索引时的最大重生成次数。
const maxPlayerNoRetries = 5

// PlayerParams 是建档选手的字段；IDCardHash 必须已按 R4 规则规范化并哈希。
type PlayerParams struct {
	Name        string
	IDType      string
	IDCardHash  string
	Gender      *string
	BirthDate   *string
	Nationality *string
}

// MatchOrCreate 在事务内按证件号哈希匹配选手，未命中则新建档案。
//
// 返回 created=true 表示本次新建。匹配命中时原样返回已有档案（不覆盖姓名等
// 字段，档案变更走后台显式操作）。player_no 撞唯一索引时重新生成重试；
// 若冲突实际来自 id_card_hash（并发下另一事务已先建档），返回已有档案。
//
// 必须在调用方事务（WithTx）内执行：查-插序列依赖事务串行化防并发重复建档。
func (s *PlayerStore) MatchOrCreate(ctx context.Context, tx *sql.Tx, p PlayerParams) (*model.Player, bool, error) {
	player, err := s.getByHashTx(ctx, tx, p.IDCardHash)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return nil, false, err
	}
	if err == nil {
		return player, false, nil
	}

	for range maxPlayerNoRetries {
		playerNo, err := generatePlayerNo()
		if err != nil {
			return nil, false, fmt.Errorf("generate player_no: %w", err)
		}
		res, err := tx.ExecContext(ctx, `
			INSERT INTO players (player_no, name, id_card_hash, id_type, gender, birth_date, nationality)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			playerNo, p.Name, p.IDCardHash, p.IDType, p.Gender, p.BirthDate, p.Nationality)
		if err == nil {
			id, err := res.LastInsertId()
			if err != nil {
				return nil, false, fmt.Errorf("last insert id of player %q: %w", playerNo, err)
			}
			player, err := s.getByIDTx(ctx, tx, id)
			if err != nil {
				return nil, false, err
			}
			return player, true, nil
		}
		if !IsUniqueViolation(err) {
			return nil, false, fmt.Errorf("insert player: %w", err)
		}
		// 唯一冲突：优先按哈希复查（并发建档已落库），否则视为 player_no 撞车重试。
		player, hashErr := s.getByHashTx(ctx, tx, p.IDCardHash)
		if hashErr == nil {
			return player, false, nil
		}
		if !errors.Is(hashErr, ErrNotFound) {
			return nil, false, hashErr
		}
	}
	return nil, false, fmt.Errorf("generate unique player_no: %w", ErrConflict)
}

// GetByID 按 id 查询选手（非事务读）。不存在返回 ErrNotFound。
func (s *PlayerStore) GetByID(ctx context.Context, id int64) (*model.Player, error) {
	player, err := scanPlayer(s.db.sql.QueryRowContext(ctx,
		`SELECT `+playerColumns+` FROM players WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query player %d: %w", id, err)
	}
	return player, nil
}

// UpdateAvatarTx 在事务内更新选手头像相对路径（T13 证件照上传后的压缩副本）。
// 旧头像文件由 service 层在事务提交后清理。
func (s *PlayerStore) UpdateAvatarTx(ctx context.Context, tx *sql.Tx, id int64, avatarPath string) error {
	res, err := tx.ExecContext(ctx,
		`UPDATE players SET avatar_path = ? WHERE id = ?`, avatarPath, id)
	if err != nil {
		return fmt.Errorf("update avatar of player %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected of player %d avatar update: %w", id, err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// GetByIDTx 在事务内按 id 查询选手档案。不存在返回 ErrNotFound。
func (s *PlayerStore) GetByIDTx(ctx context.Context, tx *sql.Tx, id int64) (*model.Player, error) {
	player, err := scanPlayer(tx.QueryRowContext(ctx,
		`SELECT `+playerColumns+` FROM players WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query player %d in tx: %w", id, err)
	}
	return player, nil
}

// SetMergedIntoTx 在事务内把重复档案指向保留档案（players.merged_into）。
// 已合并过的档案不能再次合并，条件未命中返回 ErrConflict。
func (s *PlayerStore) SetMergedIntoTx(ctx context.Context, tx *sql.Tx, id, targetID int64) error {
	res, err := tx.ExecContext(ctx,
		`UPDATE players SET merged_into = ? WHERE id = ? AND merged_into IS NULL`, targetID, id)
	if err != nil {
		return fmt.Errorf("merge player %d into %d: %w", id, targetID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected of player %d merge: %w", id, err)
	}
	if n == 0 {
		return ErrConflict
	}
	return nil
}

// MoveDataTx 在事务内把重复档案的历史数据迁移到保留档案（T16 档案合并）：
// 报名单、成绩、积分快照、总决赛资格、奖金记录。返回各表迁移行数。
//
// 目标档案已有同赛事成绩/积分等冲突数据时，唯一约束会使整笔事务回滚，
// 由 service 层提示人工核对（不做自动去重合并）。
func (s *PlayerStore) MoveDataTx(ctx context.Context, tx *sql.Tx, fromID, toID int64) (map[string]int64, error) {
	tables := []struct {
		name  string
		query string
	}{
		{"registrations", `UPDATE registrations SET player_id = ? WHERE player_id = ?`},
		{"results", `UPDATE results SET player_id = ? WHERE player_id = ?`},
		{"player_points", `UPDATE player_points SET player_id = ? WHERE player_id = ?`},
		{"final_qualifications", `UPDATE final_qualifications SET player_id = ? WHERE player_id = ?`},
		{"prizes", `UPDATE prizes SET player_id = ? WHERE player_id = ?`},
	}
	moved := make(map[string]int64, len(tables))
	for _, t := range tables {
		res, err := tx.ExecContext(ctx, t.query, toID, fromID)
		if err != nil {
			return nil, fmt.Errorf("move %s of player %d to %d: %w", t.name, fromID, toID, err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return nil, fmt.Errorf("rows affected of moving %s: %w", t.name, err)
		}
		moved[t.name] = n
	}
	return moved, nil
}

// GetByHash 按证件号哈希查询选手（非事务读）。不存在返回 ErrNotFound。
func (s *PlayerStore) GetByHash(ctx context.Context, idCardHash string) (*model.Player, error) {
	player, err := scanPlayer(s.db.sql.QueryRowContext(ctx,
		`SELECT `+playerColumns+` FROM players WHERE id_card_hash = ?`, idCardHash))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query player by hash: %w", err)
	}
	return player, nil
}

func (s *PlayerStore) getByHashTx(ctx context.Context, tx *sql.Tx, idCardHash string) (*model.Player, error) {
	player, err := scanPlayer(tx.QueryRowContext(ctx,
		`SELECT `+playerColumns+` FROM players WHERE id_card_hash = ?`, idCardHash))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query player by hash in tx: %w", err)
	}
	return player, nil
}

func (s *PlayerStore) getByIDTx(ctx context.Context, tx *sql.Tx, id int64) (*model.Player, error) {
	player, err := scanPlayer(tx.QueryRowContext(ctx,
		`SELECT `+playerColumns+` FROM players WHERE id = ?`, id))
	if err != nil {
		return nil, fmt.Errorf("query player %d in tx: %w", id, err)
	}
	return player, nil
}

// generatePlayerNo 生成 8 位字母数字随机编号（crypto/rand）。
// 唯一性由 players.player_no UNIQUE 约束兜底，撞库时由调用方重试。
func generatePlayerNo() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("read random: %w", err)
	}
	for i, b := range buf {
		buf[i] = playerNoAlphabet[int(b)%len(playerNoAlphabet)]
	}
	return string(buf), nil
}

func scanPlayer(row rowScanner) (*model.Player, error) {
	var p model.Player
	if err := row.Scan(&p.ID, &p.PlayerNo, &p.Name, &p.IDCardHash, &p.IDType,
		&p.Gender, &p.BirthDate, &p.Nationality, &p.AvatarPath, &p.MergedInto,
		&p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, err
	}
	return &p, nil
}
