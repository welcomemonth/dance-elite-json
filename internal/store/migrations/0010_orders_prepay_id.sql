-- 0010: orders 增加 prepay_id（微信支付预支付单号，T10）。
--
-- 用途：prepay 幂等——同一订单重复调预下单接口时复用已换取的 prepay_id，
-- 不向微信重复下单（真实通道下同 out_trade_no 重复下单会被拒）。
-- 条件写入（SetPrepayIDTx）：仅当订单仍为 pending_payment 且未写入过才更新。
ALTER TABLE orders ADD COLUMN prepay_id TEXT;
