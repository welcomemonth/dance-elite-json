-- 0002: 系统表（管理员 / 操作日志 / 键值配置）。
--
-- 时间戳约定（全库统一，由触发器维护）：
--   created_at / updated_at 格式为 RFC3339 本地时间，固定 +08:00（部署时区 Asia/Shanghai，
--   Docker 需设置 TZ=Asia/Shanghai）。
--   updated_at 由 AFTER UPDATE 触发器刷新；若调用方显式改写 updated_at（如数据修复），
--   触发器不再覆盖（WHEN NEW.updated_at IS OLD.updated_at）。

CREATE TABLE admins (
    id            INTEGER PRIMARY KEY,
    username      TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role          TEXT NOT NULL CHECK (role IN ('super_admin', 'admin', 'cs')),
    status        TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    last_login_at TEXT,
    created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00'),
    updated_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00')
);

CREATE TRIGGER trg_admins_updated_at
AFTER UPDATE ON admins
FOR EACH ROW
WHEN NEW.updated_at IS OLD.updated_at
BEGIN
    UPDATE admins
    SET updated_at = strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00'
    WHERE id = NEW.id;
END;

-- 敏感操作留痕，只追加不修改；admin_id 为空表示系统动作。
CREATE TABLE admin_logs (
    id          INTEGER PRIMARY KEY,
    admin_id    INTEGER REFERENCES admins(id),
    action      TEXT NOT NULL,
    target_type TEXT,
    target_id   INTEGER,
    detail      TEXT CHECK (detail IS NULL OR json_valid(detail)),
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00'),
    updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00')
);

CREATE INDEX idx_admin_logs_admin ON admin_logs(admin_id);
CREATE INDEX idx_admin_logs_target ON admin_logs(target_type, target_id);

CREATE TRIGGER trg_admin_logs_updated_at
AFTER UPDATE ON admin_logs
FOR EACH ROW
WHEN NEW.updated_at IS OLD.updated_at
BEGIN
    UPDATE admin_logs
    SET updated_at = strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00'
    WHERE id = NEW.id;
END;

-- 键值配置：value 一律存 JSON 文本（字符串也以 JSON 字符串形式存储）。
-- "key" 是 SQLite 关键字，引用列名时必须加双引号（SELECT ... WHERE "key" = ?）。
CREATE TABLE settings (
    "key"      TEXT PRIMARY KEY,
    value      TEXT NOT NULL CHECK (json_valid(value)),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00'),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00')
);

CREATE TRIGGER trg_settings_updated_at
AFTER UPDATE ON settings
FOR EACH ROW
WHEN NEW.updated_at IS OLD.updated_at
BEGIN
    UPDATE settings
    SET updated_at = strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00'
    WHERE "key" = NEW."key";
END;
