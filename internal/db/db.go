package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"

	"nagomi-core/internal/db/sqlc"

	"github.com/charmbracelet/log"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrations embed.FS

// migrationLockKey is an arbitrary fixed key for pg_advisory_lock.
const migrationLockKey = 0x6e61676f6d69 // "nagomi"

type DB struct {
	*sqlc.Queries
	log  *log.Logger
	pool *pgxpool.Pool
}

func New(dsn string) (*DB, error) {
	if dsn == "" {
		return nil, fmt.Errorf("empty dsn")
	}

	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		return nil, fmt.Errorf("pgxpool.new: %w", err)
	}

	if err := pool.Ping(context.Background()); err != nil {
		return nil, fmt.Errorf("ping: %w", err)
	}

	return &DB{
		Queries: sqlc.New(pool),
		log:     log.WithPrefix("db"),
		pool:    pool,
	}, nil
}

func (s *DB) Close() error {
	s.pool.Close()
	return nil
}

func (s *DB) Pool() *pgxpool.Pool {
	return s.pool
}

func RunMigrations(dsn string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("failed to open database for migrations: %w", err)
	}
	defer func() { _ = db.Close() }()

	goose.SetBaseFS(migrations)

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("failed to set goose dialect: %w", err)
	}

	// Serialize concurrent migrators (parallel test packages, multiple
	// instances booting) with a session-level advisory lock.
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("failed to acquire migration lock connection: %w", err)
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", migrationLockKey); err != nil {
		return fmt.Errorf("failed to acquire migration lock: %w", err)
	}
	defer func() { _, _ = conn.ExecContext(ctx, "SELECT pg_advisory_unlock($1)", migrationLockKey) }()

	if err := goose.Up(db, "migrations"); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	return nil
}
