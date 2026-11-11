-- 0009: 全系统同时最多一个 active 赛季（开发计划 T6，审核 #4 的 TOCTOU 收口）。
--
-- service 层先查 HasOtherActive 给出友好错误提示，真正的并发/重复激活由本
-- 部分唯一索引兜底：两个请求同时通过预检时，后提交者违反约束被拒（40901）。
-- 赛事"至少 1 个 enabled 组别才允许发布"的窗口更小（发布操作低频且仅管理员），
-- 由发布事务内 COUNT 校验收口，不加索引。

CREATE UNIQUE INDEX uq_seasons_single_active ON seasons(status) WHERE status = 'active';
