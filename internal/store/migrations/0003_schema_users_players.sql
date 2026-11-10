-- 0003: 用户与选手档案。
--
-- 选手唯一标识是系统生成的 player_no（8 位字母数字），id 为内部主键；
-- 证件号只存 HMAC-SHA256 哈希（服务端密钥），全库无明文，判重/匹配按哈希重算比对。

CREATE TABLE users (
    id            INTEGER PRIMARY KEY,
    openid        TEXT NOT NULL UNIQUE,
    unionid       TEXT,
    nickname      TEXT,
    avatar_url    TEXT,
    phone         TEXT,
    status        TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'blocked')),
    last_login_at TEXT,
    created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00'),
    updated_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00')
);

CREATE INDEX idx_users_phone ON users(phone);
CREATE INDEX idx_users_unionid ON users(unionid);

CREATE TRIGGER trg_users_updated_at
AFTER UPDATE ON users
FOR EACH ROW
WHEN NEW.updated_at IS OLD.updated_at
BEGIN
    UPDATE users
    SET updated_at = strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00'
    WHERE id = NEW.id;
END;

CREATE TABLE players (
    id           INTEGER PRIMARY KEY,
    player_no    TEXT NOT NULL UNIQUE
                     CHECK (length(player_no) = 8 AND player_no NOT GLOB '*[^A-Za-z0-9]*'),
    name         TEXT NOT NULL,
    id_card_hash TEXT UNIQUE,
    id_type      TEXT NOT NULL DEFAULT 'id_card' CHECK (id_type IN ('id_card', 'passport')),
    gender       TEXT CHECK (gender IS NULL OR gender IN ('male', 'female')),
    birth_date   TEXT,
    avatar_path  TEXT,
    merged_into  INTEGER REFERENCES players(id),
    created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00'),
    updated_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00')
);

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
