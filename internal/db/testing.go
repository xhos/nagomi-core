package db

import (
	"context"
	"os"
	"testing"

	"null-core/internal/db/sqlc"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type TestDB struct {
	*sqlc.Queries
	pool *pgxpool.Pool
	t    *testing.T
}

func SetupTestDB(t *testing.T) *TestDB {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	if err := RunMigrations(dsn); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}

	if err := pool.Ping(context.Background()); err != nil {
		t.Fatalf("failed to ping test database: %v", err)
	}

	tdb := &TestDB{
		Queries: sqlc.New(pool),
		pool:    pool,
		t:       t,
	}

	t.Cleanup(func() {
		pool.Close()
	})

	return tdb
}

func (tdb *TestDB) CreateTestUser(ctx context.Context) uuid.UUID {
	tdb.t.Helper()

	userID := uuid.New()
	email := userID.String() + "@test.local"

	_, err := tdb.CreateUser(ctx, sqlc.CreateUserParams{
		ID:    userID,
		Email: email,
	})
	if err != nil {
		tdb.t.Fatalf("failed to create test user: %v", err)
	}

	tdb.t.Cleanup(func() {
		_, _ = tdb.DeleteUser(context.Background(), userID)
	})

	return userID
}

func (tdb *TestDB) CreateTestAccount(ctx context.Context, params sqlc.CreateAccountParams) sqlc.Account {
	tdb.t.Helper()

	account, err := tdb.CreateAccount(ctx, params)
	if err != nil {
		tdb.t.Fatalf("failed to create test account: %v", err)
	}

	tdb.t.Cleanup(func() {
		_, _ = tdb.DeleteAccount(context.Background(), sqlc.DeleteAccountParams{
			ID:     account.ID,
			UserID: params.OwnerID,
		})
	})

	return account
}

func (tdb *TestDB) Pool() *pgxpool.Pool { return tdb.pool }
