-- 0005: 报名与支付域。
--
-- 状态机要点（见 docs/开发计划.md T9/T11/T12 定稿）：
--   registrations: pending_payment（提交报名即插入）→ active（支付回调）→ canceled / withdrawn
--   orders:        pending_payment（创建订单即插入，前端不可见）→ paid / canceled / refunding / refunded
--   payments:      type = pay / refund；status = success / failed / closed
--   refund_tickets: pending / processing / approved / rejected / refunded / refund_failed
--
-- 三防线：
--   1) registrations 部分唯一索引 (player_id, group_id) WHERE status IN ('pending_payment','active')
--      —— 同一人同一报名单元同时只能有一个进行中报名（含待支付窗口）；
--   2) event_groups.taken 原子条件 UPDATE 抢名额（taken < quota）；
--   3) orders (user_id, request_id) 唯一索引 —— 小程序重试复用 request_id，不会重复下单。

CREATE TABLE registrations (
    id                  INTEGER PRIMARY KEY,
    event_id            INTEGER NOT NULL REFERENCES events(id),
    group_id            INTEGER NOT NULL REFERENCES event_groups(id),
    player_id           INTEGER NOT NULL REFERENCES players(id),
    user_id             INTEGER NOT NULL REFERENCES users(id),
    dance_type          TEXT NOT NULL CHECK (dance_type IN ('classical', 'folk')),
    institution         TEXT,
    status              TEXT NOT NULL
                            CHECK (status IN ('pending_payment', 'active', 'canceled', 'withdrawn')),
    source              TEXT NOT NULL DEFAULT 'miniapp' CHECK (source IN ('miniapp', 'manual')),
    cs_ticket_id        INTEGER REFERENCES cs_tickets(id),
    photo_path          TEXT,
    audio_path          TEXT,
    program_name        TEXT,
    teacher_name        TEXT,
    contact_phone       TEXT,
    express_address     TEXT,
    work_intro          TEXT,
    special_req         TEXT,
    material_updated_at TEXT,
    created_at          TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00'),
    updated_at          TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00')
);

CREATE UNIQUE INDEX uq_registrations_in_flight
    ON registrations(player_id, group_id)
    WHERE status IN ('pending_payment', 'active');

CREATE INDEX idx_registrations_event ON registrations(event_id, status);
CREATE INDEX idx_registrations_group ON registrations(group_id, status);
CREATE INDEX idx_registrations_user ON registrations(user_id);
CREATE INDEX idx_registrations_player ON registrations(player_id);

CREATE TRIGGER trg_registrations_updated_at
AFTER UPDATE ON registrations
FOR EACH ROW
WHEN NEW.updated_at IS OLD.updated_at
BEGIN
    UPDATE registrations
    SET updated_at = strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00'
    WHERE id = NEW.id;
END;

CREATE TABLE orders (
    id              INTEGER PRIMARY KEY,
    order_no        TEXT NOT NULL UNIQUE,
    registration_id INTEGER NOT NULL REFERENCES registrations(id),
    user_id         INTEGER NOT NULL REFERENCES users(id),
    amount          INTEGER NOT NULL CHECK (amount >= 0),
    status          TEXT NOT NULL
                        CHECK (status IN ('pending_payment', 'paid', 'canceled', 'refunding', 'refunded')),
    request_id      TEXT,
    paid_at         TEXT,
    canceled_at     TEXT,
    cancel_reason   TEXT CHECK (cancel_reason IS NULL OR cancel_reason IN ('user_cancel', 'pay_failed', 'system')),
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00'),
    updated_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00')
);

-- 请求幂等键：同一用户同一 request_id 只允许一个订单（NULL 不参与判重，人工订单可为空）。
CREATE UNIQUE INDEX uq_orders_request ON orders(user_id, request_id)
    WHERE request_id IS NOT NULL;

CREATE INDEX idx_orders_registration ON orders(registration_id);
CREATE INDEX idx_orders_user ON orders(user_id);
CREATE INDEX idx_orders_status ON orders(status);

CREATE TRIGGER trg_orders_updated_at
AFTER UPDATE ON orders
FOR EACH ROW
WHEN NEW.updated_at IS OLD.updated_at
BEGIN
    UPDATE orders
    SET updated_at = strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00'
    WHERE id = NEW.id;
END;

CREATE TABLE payments (
    id             INTEGER PRIMARY KEY,
    order_id       INTEGER NOT NULL REFERENCES orders(id),
    type           TEXT NOT NULL CHECK (type IN ('pay', 'refund')),
    prepay_id      TEXT,
    transaction_id TEXT UNIQUE,
    refund_id      TEXT,
    amount         INTEGER NOT NULL CHECK (amount >= 0),
    status         TEXT NOT NULL CHECK (status IN ('success', 'failed', 'closed')),
    raw_callback   TEXT,
    notified_at    TEXT,
    created_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00'),
    updated_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00')
);

-- 退款回调按 refund_id 幂等（NULL 不参与判重）。
CREATE UNIQUE INDEX uq_payments_refund ON payments(refund_id)
    WHERE refund_id IS NOT NULL;

CREATE INDEX idx_payments_order ON payments(order_id);

CREATE TRIGGER trg_payments_updated_at
AFTER UPDATE ON payments
FOR EACH ROW
WHEN NEW.updated_at IS OLD.updated_at
BEGIN
    UPDATE payments
    SET updated_at = strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00'
    WHERE id = NEW.id;
END;

CREATE TABLE refund_tickets (
    id            INTEGER PRIMARY KEY,
    ticket_no     TEXT NOT NULL UNIQUE,
    order_id      INTEGER NOT NULL REFERENCES orders(id),
    reason        TEXT NOT NULL,
    applicant     TEXT,
    status        TEXT NOT NULL DEFAULT 'pending'
                      CHECK (status IN ('pending', 'processing', 'approved', 'rejected', 'refunded', 'refund_failed')),
    handler_id    INTEGER REFERENCES admins(id),
    reject_reason TEXT,
    handled_at    TEXT,
    created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00'),
    updated_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00')
);

CREATE INDEX idx_refund_tickets_status ON refund_tickets(status);
CREATE INDEX idx_refund_tickets_order ON refund_tickets(order_id);

CREATE TRIGGER trg_refund_tickets_updated_at
AFTER UPDATE ON refund_tickets
FOR EACH ROW
WHEN NEW.updated_at IS OLD.updated_at
BEGIN
    UPDATE refund_tickets
    SET updated_at = strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00'
    WHERE id = NEW.id;
END;

CREATE TABLE waitlists (
    id           INTEGER PRIMARY KEY,
    event_id     INTEGER NOT NULL REFERENCES events(id),
    group_id     INTEGER NOT NULL REFERENCES event_groups(id),
    dance_type   TEXT NOT NULL CHECK (dance_type IN ('classical', 'folk')),
    name         TEXT NOT NULL,
    id_card_hash TEXT,
    institution  TEXT,
    phone        TEXT NOT NULL,
    status       TEXT NOT NULL DEFAULT 'waiting'
                     CHECK (status IN ('waiting', 'notified', 'converted', 'given_up', 'removed')),
    position_no  INTEGER NOT NULL CHECK (position_no > 0),
    notified_at  TEXT,
    handled_at   TEXT,
    created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00'),
    updated_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00')
);

-- 同一报名单元同一证件号只能有一条排队中记录（无证件号登记以姓名+电话人工识别）。
CREATE UNIQUE INDEX uq_waitlists_waiting ON waitlists(event_id, group_id, id_card_hash)
    WHERE status = 'waiting' AND id_card_hash IS NOT NULL;

CREATE INDEX idx_waitlists_queue ON waitlists(event_id, group_id, status, position_no);

CREATE TRIGGER trg_waitlists_updated_at
AFTER UPDATE ON waitlists
FOR EACH ROW
WHEN NEW.updated_at IS OLD.updated_at
BEGIN
    UPDATE waitlists
    SET updated_at = strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00'
    WHERE id = NEW.id;
END;

CREATE TABLE cs_tickets (
    id                      INTEGER PRIMARY KEY,
    type                    TEXT NOT NULL CHECK (type IN ('refund', 'no_id_card', 'waitlist', 'other')),
    user_name               TEXT,
    phone                   TEXT NOT NULL,
    content                 TEXT NOT NULL,
    related_order_id        INTEGER REFERENCES orders(id),
    related_registration_id INTEGER REFERENCES registrations(id),
    status                  TEXT NOT NULL DEFAULT 'pending'
                                CHECK (status IN ('pending', 'processing', 'done')),
    remark                  TEXT,
    created_by              INTEGER REFERENCES admins(id),
    created_at              TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00'),
    updated_at              TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00')
);

CREATE INDEX idx_cs_tickets_status ON cs_tickets(status);
CREATE INDEX idx_cs_tickets_order ON cs_tickets(related_order_id);
CREATE INDEX idx_cs_tickets_registration ON cs_tickets(related_registration_id);

CREATE TRIGGER trg_cs_tickets_updated_at
AFTER UPDATE ON cs_tickets
FOR EACH ROW
WHEN NEW.updated_at IS OLD.updated_at
BEGIN
    UPDATE cs_tickets
    SET updated_at = strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00'
    WHERE id = NEW.id;
END;
