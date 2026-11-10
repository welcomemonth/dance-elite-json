// Package store 是数据访问层：封装 SQLite 连接、迁移与全部持久化操作。
//
// # 分层约定
//
// 依赖方向只允许 router -> service -> store，store 不得反向依赖上层。
// service 层在其包内定义窄接口消费 store（接口定义在消费方，
// 由 store 隐式实现），测试时用 fake 实现替换，不触碰真实数据库。
//
// # 事务边界
//
// 事务由调用方 service 层控制：跨表写操作必须收拢到单个事务内，
// store 提供 DB.WithTx 助手（BEGIN IMMEDIATE，见 tx.go），
// store 内的具体方法接受 *sql.Tx 参数参与既有事务，
// 不得自行嵌套开启事务。
//
// # 并发模型
//
// SQLite 单写入者：连接池限制为单连接（见 db.go），写操作天然串行。
// "名额抢占"类一致性要求使用原子条件 UPDATE（taken < quota），
// 禁止"先 SELECT 再 UPDATE"的两步写法。
//
// # 迁移
//
// migrations/*.sql 按文件名版本序执行，版本记录在 schema_migrations 表，
// 重复启动幂等。新增迁移：在 internal/store/migrations/ 追加 NNNN_name.sql，
// 禁止修改已发布的迁移文件。
package store
