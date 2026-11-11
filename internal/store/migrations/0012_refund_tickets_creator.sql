-- 0012: 退款工单登记人（T15）。
--
-- 客服可查看自己登记的退款工单（API 文档 11.4.1），需要记录登记人；
-- applicant 是展示用文本（用户本人 / 客服代录），不是操作人外键。
ALTER TABLE refund_tickets ADD COLUMN created_by INTEGER REFERENCES admins(id);

CREATE INDEX idx_refund_tickets_created_by ON refund_tickets(created_by);
