package db

import (
	"context"
	"testing"
	"time"

	"nagomi-core/internal/db/sqlc"
)

func TestCreateTransactionSource(t *testing.T) {
	tdb := SetupTestDB(t)
	ctx := context.Background()

	userID := tdb.CreateTestUser(ctx)
	account := tdb.CreateTestAccount(ctx, sqlc.CreateAccountParams{
		OwnerID:        userID,
		Name:           "Chequing",
		Bank:           "Test Bank",
		AnchorCurrency: "CAD",
		MainCurrency:   "CAD",
		Colors:         []string{"#1f2937", "#3b82f6", "#10b981"},
	})

	tests := []struct {
		name   string
		source int16
		want   int16
	}{
		{"unset defaults to manual", 0, 1},
		{"email", 2, 2},
		{"split", 5, 5},
	}

	manual := false
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx, err := tdb.CreateTransaction(ctx, sqlc.CreateTransactionParams{
				UserID:        userID,
				AccountID:     account.ID,
				TxDate:        time.Now(),
				TxAmountCents: 1000,
				TxCurrency:    "CAD",
				TxDirection:   2,
				Source:        tt.source,

				CategoryManuallySet: &manual,
				MerchantManuallySet: &manual,
			})
			if err != nil {
				t.Fatalf("CreateTransaction: %v", err)
			}
			if tx.Source != tt.want {
				t.Errorf("source = %d, want %d", tx.Source, tt.want)
			}
		})
	}
}
