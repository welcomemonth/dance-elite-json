// Package store 是数据访问层：封装 SQLite 连接、迁移与全部持久化操作。
//
// # 分层约定
//
// 依赖方向只允许 router -> service -> store，store 不得反向依赖上层。
// service 层在其包内定义窄接口消费 store（接口定义在消费方，
// 由 store 隐式实现），测试时用 fake 实现替换，不触碰真实数据库。
//
// # 入口与装配
//
// service 只持有 *Store（聚合根，见 store.go），不直接持有 *DB 或 *sql.DB；
// *sql.DB 仅用于迁移（main）与 store 包内部。领域 store 按域拆分
// （粒度 = 一张主表及其强关联表，如 EventStore 覆盖 events + event_groups），
// 在 store.New 内装配：
//
//	type Store struct {
//	    db     *DB
//	    Events *EventStore
//	}
//
//	func New(db *DB) *Store { return &Store{db: db, Events: NewEventStore(db)} }
//
// # 方法签名约定（T2 起所有领域 store 遵循）
//
//   - 读方法（Get/List/Count…）：只收 ctx，内部经 s.db 直接执行。
//   - 写方法（Create/Update/Delete…）：第一个参数收 *sql.Tx，
//     只能在 service 层的 Store.WithTx 事务内调用；单语句写也统一走
//     WithTx，保持“一切写操作必在事务中”的单一规则，不留两种写法。
//
// 领域 store 骨架（以 events 为例，T2 落地时照此实现）：
//
//	type EventStore struct{ db *DB }
//
//	func NewEventStore(db *DB) *EventStore { return &EventStore{db: db} }
//
//	func (s *EventStore) Get(ctx context.Context, id int64) (*model.Event, error)
//	func (s *EventStore) Create(ctx context.Context, tx *sql.Tx, e *model.Event) error
//
// # 事务边界
//
// 事务由调用方 service 层控制：跨表写操作必须收拢到单个事务内，
// store 提供 Store.WithTx 助手（BEGIN IMMEDIATE，见 tx.go），
// store 内的写方法接受 *sql.Tx 参数参与既有事务，不得自行嵌套开启事务。
//
// # 并发模型
//
// SQLite 单写入者：连接池限制为单连接（见 db.go），写操作天然串行。
// “名额抢占”类一致性要求使用原子条件 UPDATE（taken < quota），
// 禁止“先 SELECT 再 UPDATE”的两步写法。
//
// # 迁移
//
// migrations/*.sql 按文件名版本序执行，版本记录在 schema_migrations 表，
// 重复启动幂等。新增迁移：在 internal/store/migrations/ 追加 NNNN_name.sql，
// 禁止修改已发布的迁移文件。
package store
