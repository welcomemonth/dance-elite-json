-- 0006: 榜单与积分域（T2 仅建表，T28/T29/T31 使用）。
--
-- 榜单快照模型：player_points / institution_points 由成绩发布或纠错时事务内重算，
-- 小程序端只读快照；rank 为 SQLite 窗口函数同名标识，引用时统一加双引号。
-- results.award 存英文枚举键（champion / runner_up / third / elite_dancer /
-- special_gold / gold / silver），展示层转中文。

CREATE TABLE boards (
    id           INTEGER PRIMARY KEY,
    season_id    INTEGER NOT NULL REFERENCES seasons(id),
    level        TEXT NOT NULL CHECK (level IN ('U11', 'U13', 'U15')),
    dance_type   TEXT NOT NULL CHECK (dance_type IN ('classical', 'folk')),
    status       TEXT NOT NULL DEFAULT 'active',
    published_at TEXT,
    created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00'),
    updated_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00'),
    UNIQUE (season_id, level, dance_type)
);

CREATE TRIGGER trg_boards_updated_at
AFTER UPDATE ON boards
FOR EACH ROW
WHEN NEW.updated_at IS OLD.updated_at
BEGIN
    UPDATE boards
    SET updated_at = strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00'
    WHERE id = NEW.id;
END;

CREATE TABLE results (
    id                INTEGER PRIMARY KEY,
    event_id          INTEGER NOT NULL REFERENCES events(id),
    board_id          INTEGER NOT NULL REFERENCES boards(id),
    player_id         INTEGER NOT NULL REFERENCES players(id),
    institution       TEXT,
    award             TEXT NOT NULL,
    score_a           REAL,
    score_b           REAL,
    score_c           REAL,
    score_total       REAL,
    final_score_a     REAL,
    final_score_b     REAL,
    final_score_c     REAL,
    final_score_total REAL,
    points            INTEGER NOT NULL DEFAULT 0 CHECK (points >= 0),
    prize_amount      INTEGER NOT NULL DEFAULT 0 CHECK (prize_amount >= 0),
    advanced_to_final INTEGER NOT NULL DEFAULT 0 CHECK (advanced_to_final IN (0, 1)),
    remark            TEXT,
    status            TEXT NOT NULL DEFAULT 'published'
                          CHECK (status IN ('published', 'corrected', 'voided')),
    import_batch      TEXT,
    created_by        INTEGER REFERENCES admins(id),
    created_at        TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00'),
    updated_at        TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00')
);

-- 当前有效成绩唯一；纠错时旧记录改 status=corrected 保留留痕，新记录重新发布。
CREATE UNIQUE INDEX uq_results_published ON results(event_id, board_id, player_id)
    WHERE status = 'published';

CREATE INDEX idx_results_board ON results(board_id, player_id);
CREATE INDEX idx_results_event ON results(event_id);
CREATE INDEX idx_results_player ON results(player_id);

CREATE TRIGGER trg_results_updated_at
AFTER UPDATE ON results
FOR EACH ROW
WHEN NEW.updated_at IS OLD.updated_at
BEGIN
    UPDATE results
    SET updated_at = strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00'
    WHERE id = NEW.id;
END;

CREATE TABLE player_points (
    id             INTEGER PRIMARY KEY,
    season_id      INTEGER NOT NULL REFERENCES seasons(id),
    board_id       INTEGER NOT NULL REFERENCES boards(id),
    player_id      INTEGER NOT NULL REFERENCES players(id),
    total_points   INTEGER NOT NULL DEFAULT 0 CHECK (total_points >= 0),
    used_event_ids TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(used_event_ids)),
    "rank"         INTEGER CHECK ("rank" IS NULL OR "rank" > 0),
    display_name   TEXT,
    hidden         INTEGER NOT NULL DEFAULT 0 CHECK (hidden IN (0, 1)),
    computed_at    TEXT,
    created_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00'),
    updated_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00'),
    UNIQUE (season_id, board_id, player_id)
);

CREATE INDEX idx_player_points_board ON player_points(board_id, total_points);

CREATE TRIGGER trg_player_points_updated_at
AFTER UPDATE ON player_points
FOR EACH ROW
WHEN NEW.updated_at IS OLD.updated_at
BEGIN
    UPDATE player_points
    SET updated_at = strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00'
    WHERE id = NEW.id;
END;

CREATE TABLE institution_points (
    id               INTEGER PRIMARY KEY,
    season_id        INTEGER NOT NULL REFERENCES seasons(id),
    institution_name TEXT NOT NULL,
    total_points     INTEGER NOT NULL DEFAULT 0 CHECK (total_points >= 0),
    player_count     INTEGER NOT NULL DEFAULT 0 CHECK (player_count >= 0),
    "rank"           INTEGER CHECK ("rank" IS NULL OR "rank" > 0),
    computed_at      TEXT,
    created_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00'),
    updated_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00'),
    UNIQUE (season_id, institution_name)
);

CREATE INDEX idx_institution_points_board ON institution_points(season_id, total_points);

CREATE TRIGGER trg_institution_points_updated_at
AFTER UPDATE ON institution_points
FOR EACH ROW
WHEN NEW.updated_at IS OLD.updated_at
BEGIN
    UPDATE institution_points
    SET updated_at = strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00'
    WHERE id = NEW.id;
END;

CREATE TABLE final_qualifications (
    id         INTEGER PRIMARY KEY,
    season_id  INTEGER NOT NULL REFERENCES seasons(id),
    board_id   INTEGER NOT NULL REFERENCES boards(id),
    player_id  INTEGER NOT NULL REFERENCES players(id),
    source     TEXT NOT NULL CHECK (source IN ('auto_direct', 'manual_direct', 'points', 'manual')),
    reason     TEXT,
    status     TEXT NOT NULL DEFAULT 'valid' CHECK (status IN ('valid', 'canceled')),
    created_by INTEGER REFERENCES admins(id),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00'),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00')
);

-- 同一榜单同一选手只能有一条有效资格；取消后可重新增补。
CREATE UNIQUE INDEX uq_final_qualifications_valid
    ON final_qualifications(season_id, board_id, player_id)
    WHERE status = 'valid';

CREATE TRIGGER trg_final_qualifications_updated_at
AFTER UPDATE ON final_qualifications
FOR EACH ROW
WHEN NEW.updated_at IS OLD.updated_at
BEGIN
    UPDATE final_qualifications
    SET updated_at = strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00'
    WHERE id = NEW.id;
END;

CREATE TABLE prizes (
    id               INTEGER PRIMARY KEY,
    season_id        INTEGER NOT NULL REFERENCES seasons(id),
    event_id         INTEGER REFERENCES events(id),
    board_id         INTEGER REFERENCES boards(id),
    player_id        INTEGER REFERENCES players(id),
    institution_name TEXT,
    award            TEXT NOT NULL,
    amount           INTEGER NOT NULL CHECK (amount >= 0),
    status           TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'granted', 'skipped')),
    granted_at       TEXT,
    grantee_note     TEXT,
    created_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00'),
    updated_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00')
);

CREATE INDEX idx_prizes_season ON prizes(season_id, status);
CREATE INDEX idx_prizes_event ON prizes(event_id);

CREATE TRIGGER trg_prizes_updated_at
AFTER UPDATE ON prizes
FOR EACH ROW
WHEN NEW.updated_at IS OLD.updated_at
BEGIN
    UPDATE prizes
    SET updated_at = strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00'
    WHERE id = NEW.id;
END;
