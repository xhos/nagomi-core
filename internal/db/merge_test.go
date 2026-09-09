package db

import (
	"context"
	"testing"
	"time"

	"nagomi-core/internal/db/sqlc"

	"github.com/google/uuid"
)

func TestMergeAccounts(t *testing.T) {
	tdb := SetupTestDB(t)
	ctx := context.Background()

	makeAccount := func(userID uuid.UUID, name string, aliases []string) sqlc.Account {
		t.Helper()
		account := tdb.CreateTestAccount(ctx, sqlc.CreateAccountParams{
			OwnerID:        userID,
			Name:           name,
			Bank:           "Test Bank",
			AnchorCurrency: "CAD",
			MainCurrency:   "CAD",
			Colors:         []string{"#1f2937", "#3b82f6", "#10b981"},
		})
		if len(aliases) > 0 {
			if err := tdb.SetAccountAliases(ctx, sqlc.SetAccountAliasesParams{
				ID:      account.ID,
				UserID:  userID,
				Aliases: aliases,
			}); err != nil {
				t.Fatalf("SetAccountAliases: %v", err)
			}
		}
		return account
	}

	createTx := func(accountID int64) int64 {
		t.Helper()
		var id int64
		err := tdb.Pool().QueryRow(ctx, `
			INSERT INTO transactions (account_id, tx_date, tx_amount_cents, tx_currency, tx_direction)
			VALUES ($1, $2, 10000, 'CAD', 1)
			RETURNING id
		`, accountID, time.Now()).Scan(&id)
		if err != nil {
			t.Fatalf("failed to create transaction: %v", err)
		}
		return id
	}

	txAccountID := func(txID int64) int64 {
		t.Helper()
		var accountID int64
		if err := tdb.Pool().QueryRow(ctx, `SELECT account_id FROM transactions WHERE id = $1`, txID).Scan(&accountID); err != nil {
			t.Fatalf("failed to get transaction account_id: %v", err)
		}
		return accountID
	}

	t.Run("transactions move to primary", func(t *testing.T) {
		userID := tdb.CreateTestUser(ctx)
		primary := makeAccount(userID, "1234", nil)
		secondary := makeAccount(userID, "1235", nil)

		tx1 := createTx(secondary.ID)
		tx2 := createTx(secondary.ID)

		moved, err := tdb.MoveAccountTransactions(ctx, sqlc.MoveAccountTransactionsParams{
			PrimaryID:   primary.ID,
			SecondaryID: secondary.ID,
		})
		if err != nil {
			t.Fatalf("MoveAccountTransactions: %v", err)
		}
		if moved != 2 {
			t.Errorf("moved %d transactions, want 2", moved)
		}
		for _, txID := range []int64{tx1, tx2} {
			if got := txAccountID(txID); got != primary.ID {
				t.Errorf("tx %d: account_id = %d, want %d", txID, got, primary.ID)
			}
		}
	})

	t.Run("secondary name and aliases absorbed into primary", func(t *testing.T) {
		userID := tdb.CreateTestUser(ctx)
		primary := makeAccount(userID, "1234", []string{"old-1234"})
		_ = makeAccount(userID, "1235", []string{"old-1235", "visa-gold"})

		// Simulate what the service does: merge aliases
		merged := []string{"old-1234", "1235", "old-1235", "visa-gold"}
		if err := tdb.SetAccountAliases(ctx, sqlc.SetAccountAliasesParams{
			ID:      primary.ID,
			UserID:  userID,
			Aliases: merged,
		}); err != nil {
			t.Fatalf("SetAccountAliases: %v", err)
		}

		row, err := tdb.GetAccount(ctx, sqlc.GetAccountParams{UserID: userID, ID: primary.ID})
		if err != nil {
			t.Fatalf("GetAccount: %v", err)
		}

		aliasSet := make(map[string]struct{}, len(row.Account.Aliases))
		for _, a := range row.Account.Aliases {
			aliasSet[a] = struct{}{}
		}
		for _, expected := range []string{"old-1234", "1235", "old-1235", "visa-gold"} {
			if _, ok := aliasSet[expected]; !ok {
				t.Errorf("alias %q not found on primary after merge", expected)
			}
		}
	})

	t.Run("secondary is deleted after merge", func(t *testing.T) {
		userID := tdb.CreateTestUser(ctx)
		primary := makeAccount(userID, "1234", nil)
		secondary := makeAccount(userID, "1235", nil)

		_, err := tdb.MoveAccountTransactions(ctx, sqlc.MoveAccountTransactionsParams{
			PrimaryID:   primary.ID,
			SecondaryID: secondary.ID,
		})
		if err != nil {
			t.Fatalf("MoveAccountTransactions: %v", err)
		}

		affected, err := tdb.DeleteAccount(ctx, sqlc.DeleteAccountParams{
			ID:     secondary.ID,
			UserID: userID,
		})
		if err != nil {
			t.Fatalf("DeleteAccount: %v", err)
		}
		if affected != 1 {
			t.Errorf("DeleteAccount affected %d rows, want 1", affected)
		}

		if _, err := tdb.GetAccount(ctx, sqlc.GetAccountParams{UserID: userID, ID: secondary.ID}); err == nil {
			t.Error("secondary account should not exist after deletion")
		}
	})

	t.Run("transactions on secondary survive deletion via cascade does not apply - moved first", func(t *testing.T) {
		userID := tdb.CreateTestUser(ctx)
		primary := makeAccount(userID, "1234", nil)
		secondary := makeAccount(userID, "1235", nil)

		txID := createTx(secondary.ID)

		if _, err := tdb.MoveAccountTransactions(ctx, sqlc.MoveAccountTransactionsParams{
			PrimaryID:   primary.ID,
			SecondaryID: secondary.ID,
		}); err != nil {
			t.Fatalf("MoveAccountTransactions: %v", err)
		}

		if _, err := tdb.DeleteAccount(ctx, sqlc.DeleteAccountParams{
			ID: secondary.ID, UserID: userID,
		}); err != nil {
			t.Fatalf("DeleteAccount: %v", err)
		}

		// Transaction should still exist, now on primary
		if got := txAccountID(txID); got != primary.ID {
			t.Errorf("tx account_id = %d after merge+delete, want %d", got, primary.ID)
		}
	})

	t.Run("move is a no-op when secondary has no transactions", func(t *testing.T) {
		userID := tdb.CreateTestUser(ctx)
		primary := makeAccount(userID, "1234", nil)
		secondary := makeAccount(userID, "1235", nil)

		moved, err := tdb.MoveAccountTransactions(ctx, sqlc.MoveAccountTransactionsParams{
			PrimaryID:   primary.ID,
			SecondaryID: secondary.ID,
		})
		if err != nil {
			t.Fatalf("MoveAccountTransactions: %v", err)
		}
		if moved != 0 {
			t.Errorf("moved %d transactions, want 0", moved)
		}
	})

	t.Run("primary transactions are unaffected by move", func(t *testing.T) {
		userID := tdb.CreateTestUser(ctx)
		primary := makeAccount(userID, "1234", nil)
		secondary := makeAccount(userID, "1235", nil)

		primaryTx := createTx(primary.ID)
		secondaryTx := createTx(secondary.ID)

		if _, err := tdb.MoveAccountTransactions(ctx, sqlc.MoveAccountTransactionsParams{
			PrimaryID:   primary.ID,
			SecondaryID: secondary.ID,
		}); err != nil {
			t.Fatalf("MoveAccountTransactions: %v", err)
		}

		if got := txAccountID(primaryTx); got != primary.ID {
			t.Errorf("primary tx moved unexpectedly to account %d", got)
		}
		if got := txAccountID(secondaryTx); got != primary.ID {
			t.Errorf("secondary tx not moved, still on account %d", got)
		}
	})
}
