package store

import (
	"context"
	"database/sql"
)

// Store 是 service 层的数据访问入口，聚合全部领域 store。
//
// 装配：组合根（main）以 store.New(db) 构造一次；领域 store 在 New 内
// 构造并挂在 Store 上（见 doc.go 示例）。service 构造函数只接收 *Store，
// 不直接持有 *DB 或 *sql.DB。
type Store struct {
	db *DB

	// 领域 store 按域组装；service 只依赖 *Store，不直接持有 *DB。
	Users         *UserStore
	Players       *PlayerStore
	Admins        *AdminStore
	Settings      *SettingsStore
	Seasons       *SeasonStore
	Events        *EventStore
	Groups        *EventGroupStore
	Registrations *RegistrationStore
	Orders        *OrderStore
	Payments      *PaymentStore
	RefundTickets *RefundTicketStore
	Waitlists     *WaitlistStore
	CSTickets     *CSTicketStore
}

// New 构造数据访问层聚合根。
func New(db *DB) *Store {
	return &Store{
		db:            db,
		Users:         NewUserStore(db),
		Players:       NewPlayerStore(db),
		Admins:        NewAdminStore(db),
		Settings:      NewSettingsStore(db),
		Seasons:       NewSeasonStore(db),
		Events:        NewEventStore(db),
		Groups:        NewEventGroupStore(db),
		Registrations: NewRegistrationStore(db),
		Orders:        NewOrderStore(db),
		Payments:      NewPaymentStore(db),
		RefundTickets: NewRefundTicketStore(db),
		Waitlists:     NewWaitlistStore(db),
		CSTickets:     NewCSTicketStore(db),
	}
}

// WithTx 开启事务并执行 fn，事务边界由调用方（service 层）控制。
// 约定：store 的写方法只在 WithTx 内调用（见 doc.go），
// 读方法不参与事务、直接执行。
func (s *Store) WithTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	return s.db.WithTx(ctx, fn)
}
