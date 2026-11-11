-- 0014: 资料催收联系记录（开发计划 T17 / API 文档 12.3–12.4）。
--
-- 每次「标记已联系」追加一条记录（催收时间 + 备注留痕，不覆盖历史）；
-- 名单的 contacted 标记由该表是否存在记录实时判断，不在 registrations 上存状态。

CREATE TABLE material_contacts (
    id              INTEGER PRIMARY KEY,
    registration_id INTEGER NOT NULL REFERENCES registrations(id),
    note            TEXT,
    created_by      INTEGER REFERENCES admins(id),
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime') || '+08:00')
);

CREATE INDEX idx_material_contacts_registration ON material_contacts(registration_id);
