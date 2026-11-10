package store

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// DB 是 SQLite 数据访问层的入口，封装 *sql.DB 句柄。
//
// 说明：
//   - 驱动使用 modernc.org/sqlite（纯 Go 实现），Docker 交叉编译无需 CGO；
//   - SQLite 单写入者模型，连接池限制为单连接，由驱动层串行化写操作，
//     业务层只需在事务中控制一致性，无需自行加锁。
type DB struct {
	sql *sql.DB
}

// pragmas 打开连接后强制设置的 SQLite PRAGMA。
// 每项单独 Exec，连接池中的每个连接都会应用（modernc 支持连接级 PRAGMA）。
const pragmas = `
PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;
PRAGMA busy_timeout = 5000;
PRAGMA synchronous = NORMAL;
`

// Open 打开（必要时创建）SQLite 数据库文件并应用 PRAGMA 设置。
//
// path 为数据库文件路径，如 "data/dance-elite.db"；":memory:" 用于测试。
// 父目录须已存在（由部署脚本或调用方保证）。
func Open(path string) (*DB, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)", path)
	// modernc 的 _pragma DSN 参数在打开时设置一次；连接池复用连接时仍需
	// 通过连接钩子保证每个连接都启用外键，这里用单连接池 + 显式 Exec 兜底。
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}

	// SQLite 单写入者：写连接串行化即可，限制为 1 避免 "database is locked"。
	// 不回收连接：MaxOpenConns=1 时连接重建在 :memory: 下会丢数据。
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(0)

	if _, err := sqlDB.Exec(pragmas); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("apply pragmas: %w", err)
	}
	if err := sqlDB.Ping(); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ping sqlite %s: %w", path, err)
	}
	return &DB{sql: sqlDB}, nil
}

// SQL 返回底层句柄，供迁移框架与测试使用。
// 业务代码不要直接持有 *sql.DB，统一经 DB 与具体 store 方法访问。
func (d *DB) SQL() *sql.DB { return d.sql }

// Close 关闭数据库连接。
func (d *DB) Close() error { return d.sql.Close() }
