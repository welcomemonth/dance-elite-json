-- 0008: admins 增加真实姓名。
--
-- username 是登录账号；name 是后台展示用的真实姓名（账号管理创建时录入）。
-- 历史数据（此前仅由启动种子创建的超管）没有姓名，由 Seed 在启动时补默认显示名。

ALTER TABLE admins ADD COLUMN name TEXT;
