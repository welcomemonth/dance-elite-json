-- 0004: 赛季 / 赛事 / 组别×舞种配置。
--
-- level 枚举：赛事 level = grade_a（甲级赛）/ super（超级赛）/ final（年度总决赛）；
--             组别 level = U11 / U13 / U15。
-- dance_type 枚举：classical（古典舞）/ folk（民族民间舞），中文仅展示层转换。
-- 时间线 5 节点由服务层校验先后（T6），此处再加一层非空时的排他顺序约束兜底。

CREATE TABLE seasons (
    id          INTEGER PRIMARY KEY,
    name        TEXT NOT NULL,
    start_date  TEXT,
    end_date    TEXT,
    status      TEXT NOT NULL DEFAULT 'upcoming' CHECK (status IN ('upcoming', 'active', 'archived')),
    group_rules TEXT CHECK (group_rules IS NULL OR json_valid(group_rules)),
    archived_at TEXT,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00'),
    updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00'),
    CHECK (start_date IS NULL OR end_date IS NULL OR start_date <= end_date)
);

CREATE INDEX idx_seasons_status ON seasons(status);

CREATE TRIGGER trg_seasons_updated_at
AFTER UPDATE ON seasons
FOR EACH ROW
WHEN NEW.updated_at IS OLD.updated_at
BEGIN
    UPDATE seasons
    SET updated_at = strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00'
    WHERE id = NEW.id;
END;

CREATE TABLE events (
    id                  INTEGER PRIMARY KEY,
    season_id           INTEGER NOT NULL REFERENCES seasons(id),
    name                TEXT NOT NULL,
    level               TEXT NOT NULL CHECK (level IN ('grade_a', 'super', 'final')),
    city                TEXT,
    venue               TEXT,
    event_date          TEXT,
    fee                 INTEGER NOT NULL DEFAULT 0 CHECK (fee >= 0),
    cover_path          TEXT,
    reg_start_at        TEXT,
    reg_end_at          TEXT,
    lottery_at          TEXT,
    material_check_at   TEXT,
    start_at            TEXT,
    status              TEXT NOT NULL DEFAULT 'draft'
                            CHECK (status IN ('draft', 'unpublished', 'registering', 'reg_closed', 'ongoing', 'finished', 'offline')),
    score_rules         TEXT CHECK (score_rules IS NULL OR json_valid(score_rules)),
    prize_rules         TEXT CHECK (prize_rules IS NULL OR json_valid(prize_rules)),
    refund_policy       TEXT,
    work_duration_limit TEXT,
    process_desc        TEXT,
    honor_desc          TEXT,
    exclude_from_prize  INTEGER NOT NULL DEFAULT 0 CHECK (exclude_from_prize IN (0, 1)),
    audio_review_token  TEXT UNIQUE
                            CHECK (audio_review_token IS NULL
                                   OR (length(audio_review_token) = 8
                                       AND audio_review_token NOT GLOB '*[^A-Za-z0-9]*')),
    created_at          TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00'),
    updated_at          TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00'),
    CHECK (reg_start_at IS NULL OR reg_end_at IS NULL OR reg_start_at < reg_end_at),
    CHECK (reg_end_at IS NULL OR lottery_at IS NULL OR reg_end_at < lottery_at),
    CHECK (lottery_at IS NULL OR material_check_at IS NULL OR lottery_at < material_check_at),
    CHECK (material_check_at IS NULL OR start_at IS NULL OR material_check_at < start_at)
);

CREATE INDEX idx_events_season ON events(season_id);
CREATE INDEX idx_events_status ON events(status);

CREATE TRIGGER trg_events_updated_at
AFTER UPDATE ON events
FOR EACH ROW
WHEN NEW.updated_at IS OLD.updated_at
BEGIN
    UPDATE events
    SET updated_at = strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00'
    WHERE id = NEW.id;
END;

-- 一行 = 一个可报名单元（赛事 × 组别 × 舞种），名额按此粒度设置。
-- taken 为已占名额计数（提交报名事务内原子自增，失败回滚；取消报名时自减），
-- 抢占以 UPDATE event_groups SET taken = taken + 1 WHERE id = ? AND taken < quota 为准。
CREATE TABLE event_groups (
    id              INTEGER PRIMARY KEY,
    event_id        INTEGER NOT NULL REFERENCES events(id),
    level           TEXT NOT NULL CHECK (level IN ('U11', 'U13', 'U15')),
    dance_type      TEXT NOT NULL CHECK (dance_type IN ('classical', 'folk')),
    birth_year_from INTEGER NOT NULL,
    birth_year_to   INTEGER NOT NULL,
    quota           INTEGER NOT NULL CHECK (quota >= 0),
    taken           INTEGER NOT NULL DEFAULT 0 CHECK (taken >= 0),
    enabled         INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00'),
    updated_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00'),
    UNIQUE (event_id, level, dance_type),
    CHECK (birth_year_from <= birth_year_to)
);

CREATE TRIGGER trg_event_groups_updated_at
AFTER UPDATE ON event_groups
FOR EACH ROW
WHEN NEW.updated_at IS OLD.updated_at
BEGIN
    UPDATE event_groups
    SET updated_at = strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00'
    WHERE id = NEW.id;
END;
