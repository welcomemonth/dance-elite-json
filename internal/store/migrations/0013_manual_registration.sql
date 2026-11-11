-- 0013: 人工报名通道（T16）。
--
-- 1) players.id_type 扩展为 id_card / passport / hk_macau / other（后台人工录入的
--    护照、港澳台证件与其他证件），并新增国籍/地区字段；SQLite 无法修改 CHECK，
--    按标准重建流程：建新表 → 复制 → DROP 旧表 → RENAME（迁移框架在事务外
--    临时关闭外键校验，子表引用按表名 "players" 在 RENAME 后自然指向新表）。
-- 2) orders.source 区分在线支付与后台人工录入（线下收款），供后台订单筛选与展示。

CREATE TABLE players_rebuild (
    id           INTEGER PRIMARY KEY,
    player_no    TEXT NOT NULL UNIQUE
                     CHECK (length(player_no) = 8 AND player_no NOT GLOB '*[^A-Za-z0-9]*'),
    name         TEXT NOT NULL,
    id_card_hash TEXT UNIQUE,
    id_type      TEXT NOT NULL DEFAULT 'id_card'
                     CHECK (id_type IN ('id_card', 'passport', 'hk_macau', 'other')),
    gender       TEXT CHECK (gender IS NULL OR gender IN ('male', 'female')),
    birth_date   TEXT,
    nationality  TEXT,
    avatar_path  TEXT,
    merged_into  INTEGER REFERENCES players(id),
    created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00'),
    updated_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00')
);

INSERT INTO players_rebuild (id, player_no, name, id_card_hash, id_type, gender,
    birth_date, avatar_path, merged_into, created_at, updated_at)
SELECT id, player_no, name, id_card_hash, id_type, gender,
    birth_date, avatar_path, merged_into, created_at, updated_at
FROM players;

DROP TABLE players;
ALTER TABLE players_rebuild RENAME TO players;

CREATE INDEX idx_players_merged_into ON players(merged_into);

CREATE TRIGGER trg_players_updated_at
AFTER UPDATE ON players
FOR EACH ROW
WHEN NEW.updated_at IS OLD.updated_at
BEGIN
    UPDATE players
    SET updated_at = strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00'
    WHERE id = NEW.id;
END;

ALTER TABLE orders ADD COLUMN source TEXT NOT NULL DEFAULT 'miniapp'
    CHECK (source IN ('miniapp', 'manual'));
