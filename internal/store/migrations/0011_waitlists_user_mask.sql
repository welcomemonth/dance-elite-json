-- 0011: 候补队列补充会话归属与脱敏证件号（开发计划 T14）。
--
-- user_id：候补以微信会话提交，「我的候补」与用户自助取消（API 文档 5.2/5.3）
--   需要按提交人过滤；原表结构未记录归属。
-- id_card_masked：后台候补列表展示脱敏证件号（API 文档 12.1）。证件号只存
--   HMAC 哈希、不可逆出掩码，故在提交时保存脱敏值（掩码本身不含完整明文）。

ALTER TABLE waitlists ADD COLUMN user_id INTEGER REFERENCES users(id);
ALTER TABLE waitlists ADD COLUMN id_card_masked TEXT;

CREATE INDEX idx_waitlists_user ON waitlists(user_id);
