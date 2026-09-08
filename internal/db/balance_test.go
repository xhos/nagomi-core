package db

import (
	"context"
	"testing"
	"time"

	"null-core/internal/db/sqlc"
)

// TestSyncAccountBalances tests the balance calculation query.
// The anchor-based system calculates balances forward from anchor for transactions
// on or after anchor_date, and backward for transactions before anchor_date.
func TestSyncAccountBalances(t *testing.T) {
	tdb := SetupTestDB(t)
	ctx := context.Background()

	// Helper to create transactions directly via SQL (bypassing service layer)
	createTx := func(accountID int64, date time.Time, cents int64, direction int16) int64 {
		t.Helper()
		var id int64
		err := tdb.Pool().QueryRow(ctx, `
			INSERT INTO transactions (account_id, tx_date, tx_amount_cents, tx_currency, tx_direction)
			VALUES ($1, $2, $3, 'CAD', $4)
			RETURNING id
		`, accountID, date, cents, direction).Scan(&id)
		if err != nil {
			t.Fatalf("failed to create transaction: %v", err)
		}
		return id
	}

	// Helper to get balance_after_cents for a transaction
	getBalance := func(txID int64) *int64 {
		t.Helper()
		var balance *int64
		err := tdb.Pool().QueryRow(ctx, `
			SELECT balance_after_cents FROM transactions WHERE id = $1
		`, txID).Scan(&balance)
		if err != nil {
			t.Fatalf("failed to get balance: %v", err)
		}
		return balance
	}

	// Direction constants (matching the proto enum)
	const (
		incoming int16 = 1
		outgoing int16 = 2
	)

	t.Run("forward calculation - all transactions after anchor", func(t *testing.T) {
		userID := tdb.CreateTestUser(ctx)
		anchorDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)

		account := tdb.CreateTestAccount(ctx, sqlc.CreateAccountParams{
			OwnerID:            userID,
			Name:               "test-forward",
			Bank:               "Test Bank",
			AnchorBalanceCents: 100000, // $1000.00
			AnchorCurrency:     "CAD",
			MainCurrency:       "CAD",
			Colors:             []string{"#1f2937", "#3b82f6", "#10b981"},
		})

		// Update anchor_date (CreateAccount uses CURRENT_DATE)
		_, err := tdb.Pool().Exec(ctx, `UPDATE accounts SET anchor_date = $1 WHERE id = $2`, anchorDate, account.ID)
		if err != nil {
			t.Fatalf("failed to update anchor_date: %v", err)
		}

		// Create transactions after anchor
		tx1 := createTx(account.ID, anchorDate.Add(24*time.Hour), 20000, incoming) // +$200 on Jan 16
		tx2 := createTx(account.ID, anchorDate.Add(48*time.Hour), 5000, outgoing)  // -$50 on Jan 17
		tx3 := createTx(account.ID, anchorDate.Add(72*time.Hour), 10000, incoming) // +$100 on Jan 18

		// Sync balances
		err = tdb.SyncAccountBalances(ctx, account.ID)
		if err != nil {
			t.Fatalf("SyncAccountBalances failed: %v", err)
		}

		// Verify: anchor=1000, +200=1200, -50=1150, +100=1250
		assertBalance(t, getBalance(tx1), 120000) // $1200
		assertBalance(t, getBalance(tx2), 115000) // $1150
		assertBalance(t, getBalance(tx3), 125000) // $1250
	})

	t.Run("backward calculation - all transactions before anchor", func(t *testing.T) {
		userID := tdb.CreateTestUser(ctx)
		anchorDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)

		account := tdb.CreateTestAccount(ctx, sqlc.CreateAccountParams{
			OwnerID:            userID,
			Name:               "test-backward",
			Bank:               "Test Bank",
			AnchorBalanceCents: 100000, // $1000.00
			AnchorCurrency:     "CAD",
			MainCurrency:       "CAD",
			Colors:             []string{"#1f2937", "#3b82f6", "#10b981"},
		})

		_, err := tdb.Pool().Exec(ctx, `UPDATE accounts SET anchor_date = $1 WHERE id = $2`, anchorDate, account.ID)
		if err != nil {
			t.Fatalf("failed to update anchor_date: %v", err)
		}

		// Create transactions before anchor
		// Working backward: anchor=1000, so to find balance after earlier tx,
		// we subtract the effect of transactions that came after it
		tx1 := createTx(account.ID, anchorDate.Add(-5*24*time.Hour), 10000, incoming) // +$100 on Jan 10
		tx2 := createTx(account.ID, anchorDate.Add(-3*24*time.Hour), 5000, outgoing)  // -$50 on Jan 12

		err = tdb.SyncAccountBalances(ctx, account.ID)
		if err != nil {
			t.Fatalf("SyncAccountBalances failed: %v", err)
		}

		// Verify backward calculation:
		// Balance at anchor = 1000
		// Balance after Jan 12 = 1000 (no transactions between Jan 12 and anchor)
		// Balance after Jan 10 = 1000 - (-50) = 1050 (undo the Jan 12 withdrawal effect)
		assertBalance(t, getBalance(tx2), 100000) // $1000 after Jan 12
		assertBalance(t, getBalance(tx1), 105000) // $1050 after Jan 10
	})

	t.Run("mixed - transactions before and after anchor", func(t *testing.T) {
		userID := tdb.CreateTestUser(ctx)
		anchorDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)

		account := tdb.CreateTestAccount(ctx, sqlc.CreateAccountParams{
			OwnerID:            userID,
			Name:               "test-mixed",
			Bank:               "Test Bank",
			AnchorBalanceCents: 100000,
			AnchorCurrency:     "CAD",
			MainCurrency:       "CAD",
			Colors:             []string{"#1f2937", "#3b82f6", "#10b981"},
		})

		_, err := tdb.Pool().Exec(ctx, `UPDATE accounts SET anchor_date = $1 WHERE id = $2`, anchorDate, account.ID)
		if err != nil {
			t.Fatalf("failed to update anchor_date: %v", err)
		}

		// Before anchor
		txBefore := createTx(account.ID, anchorDate.Add(-2*24*time.Hour), 5000, outgoing) // -$50 on Jan 13

		// After anchor
		txAfter := createTx(account.ID, anchorDate.Add(2*24*time.Hour), 20000, incoming) // +$200 on Jan 17

		err = tdb.SyncAccountBalances(ctx, account.ID)
		if err != nil {
			t.Fatalf("SyncAccountBalances failed: %v", err)
		}

		// Before: balance after Jan 13 = 1000 (no tx between Jan 13 and anchor)
		// After: balance after Jan 17 = 1000 + 200 = 1200
		assertBalance(t, getBalance(txBefore), 100000) // $1000
		assertBalance(t, getBalance(txAfter), 120000)  // $1200
	})

	t.Run("transaction exactly on anchor date - treated as after", func(t *testing.T) {
		userID := tdb.CreateTestUser(ctx)
		anchorDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)

		account := tdb.CreateTestAccount(ctx, sqlc.CreateAccountParams{
			OwnerID:            userID,
			Name:               "test-on-anchor",
			Bank:               "Test Bank",
			AnchorBalanceCents: 100000,
			AnchorCurrency:     "CAD",
			MainCurrency:       "CAD",
			Colors:             []string{"#1f2937", "#3b82f6", "#10b981"},
		})

		_, err := tdb.Pool().Exec(ctx, `UPDATE accounts SET anchor_date = $1 WHERE id = $2`, anchorDate, account.ID)
		if err != nil {
			t.Fatalf("failed to update anchor_date: %v", err)
		}

		// Transaction exactly on anchor date (>= means it goes to after_anchor CTE)
		txOnAnchor := createTx(account.ID, anchorDate, 15000, incoming) // +$150 on Jan 15

		err = tdb.SyncAccountBalances(ctx, account.ID)
		if err != nil {
			t.Fatalf("SyncAccountBalances failed: %v", err)
		}

		// Forward calculation: anchor + 150 = 1150
		assertBalance(t, getBalance(txOnAnchor), 115000)
	})

	t.Run("same date different IDs - ID ordering", func(t *testing.T) {
		userID := tdb.CreateTestUser(ctx)
		anchorDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)

		account := tdb.CreateTestAccount(ctx, sqlc.CreateAccountParams{
			OwnerID:            userID,
			Name:               "test-same-date",
			Bank:               "Test Bank",
			AnchorBalanceCents: 100000,
			AnchorCurrency:     "CAD",
			MainCurrency:       "CAD",
			Colors:             []string{"#1f2937", "#3b82f6", "#10b981"},
		})

		_, err := tdb.Pool().Exec(ctx, `UPDATE accounts SET anchor_date = $1 WHERE id = $2`, anchorDate, account.ID)
		if err != nil {
			t.Fatalf("failed to update anchor_date: %v", err)
		}

		// Same date, created in order (IDs will be sequential)
		sameDate := anchorDate.Add(24 * time.Hour)
		tx1 := createTx(account.ID, sameDate, 10000, incoming) // +$100, lower ID
		tx2 := createTx(account.ID, sameDate, 5000, outgoing)  // -$50, higher ID

		err = tdb.SyncAccountBalances(ctx, account.ID)
		if err != nil {
			t.Fatalf("SyncAccountBalances failed: %v", err)
		}

		// tx1 (lower ID) processed first: 1000 + 100 = 1100
		// tx2 (higher ID) processed second: 1100 - 50 = 1050
		assertBalance(t, getBalance(tx1), 110000) // $1100
		assertBalance(t, getBalance(tx2), 105000) // $1050
	})

	t.Run("empty account - no transactions", func(t *testing.T) {
		userID := tdb.CreateTestUser(ctx)

		account := tdb.CreateTestAccount(ctx, sqlc.CreateAccountParams{
			OwnerID:            userID,
			Name:               "test-empty",
			Bank:               "Test Bank",
			AnchorBalanceCents: 50000,
			AnchorCurrency:     "CAD",
			MainCurrency:       "CAD",
			Colors:             []string{"#1f2937", "#3b82f6", "#10b981"},
		})

		// Should not error on empty account
		err := tdb.SyncAccountBalances(ctx, account.ID)
		if err != nil {
			t.Fatalf("SyncAccountBalances failed on empty account: %v", err)
		}
	})

	t.Run("negative anchor balance", func(t *testing.T) {
		userID := tdb.CreateTestUser(ctx)
		anchorDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)

		account := tdb.CreateTestAccount(ctx, sqlc.CreateAccountParams{
			OwnerID:            userID,
			Name:               "test-negative",
			Bank:               "Test Bank",
			AnchorBalanceCents: -50000, // -$500 (debt)
			AnchorCurrency:     "CAD",
			MainCurrency:       "CAD",
			Colors:             []string{"#1f2937", "#3b82f6", "#10b981"},
		})

		_, err := tdb.Pool().Exec(ctx, `UPDATE accounts SET anchor_date = $1 WHERE id = $2`, anchorDate, account.ID)
		if err != nil {
			t.Fatalf("failed to update anchor_date: %v", err)
		}

		tx1 := createTx(account.ID, anchorDate.Add(24*time.Hour), 20000, incoming) // +$200

		err = tdb.SyncAccountBalances(ctx, account.ID)
		if err != nil {
			t.Fatalf("SyncAccountBalances failed: %v", err)
		}

		// -500 + 200 = -300
		assertBalance(t, getBalance(tx1), -30000)
	})

	t.Run("balance currency set from main_currency", func(t *testing.T) {
		userID := tdb.CreateTestUser(ctx)
		anchorDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)

		account := tdb.CreateTestAccount(ctx, sqlc.CreateAccountParams{
			OwnerID:            userID,
			Name:               "test-currency",
			Bank:               "Test Bank",
			AnchorBalanceCents: 100000,
			AnchorCurrency:     "USD",
			MainCurrency:       "USD",
			Colors:             []string{"#1f2937", "#3b82f6", "#10b981"},
		})

		_, err := tdb.Pool().Exec(ctx, `UPDATE accounts SET anchor_date = $1 WHERE id = $2`, anchorDate, account.ID)
		if err != nil {
			t.Fatalf("failed to update anchor_date: %v", err)
		}

		txID := createTx(account.ID, anchorDate.Add(24*time.Hour), 5000, incoming)

		err = tdb.SyncAccountBalances(ctx, account.ID)
		if err != nil {
			t.Fatalf("SyncAccountBalances failed: %v", err)
		}

		var currency *string
		err = tdb.Pool().QueryRow(ctx, `SELECT balance_currency FROM transactions WHERE id = $1`, txID).Scan(&currency)
		if err != nil {
			t.Fatalf("failed to get currency: %v", err)
		}

		if currency == nil || *currency != "USD" {
			t.Errorf("expected balance_currency 'USD', got %v", currency)
		}
	})

	t.Run("multiple transactions before anchor - backward calculation chain", func(t *testing.T) {
		userID := tdb.CreateTestUser(ctx)
		anchorDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)

		account := tdb.CreateTestAccount(ctx, sqlc.CreateAccountParams{
			OwnerID:            userID,
			Name:               "test-backward-chain",
			Bank:               "Test Bank",
			AnchorBalanceCents: 100000, // $1000
			AnchorCurrency:     "CAD",
			MainCurrency:       "CAD",
			Colors:             []string{"#1f2937", "#3b82f6", "#10b981"},
		})

		_, err := tdb.Pool().Exec(ctx, `UPDATE accounts SET anchor_date = $1 WHERE id = $2`, anchorDate, account.ID)
		if err != nil {
			t.Fatalf("failed to update anchor_date: %v", err)
		}

		// Create chain of transactions before anchor
		tx1 := createTx(account.ID, anchorDate.Add(-4*24*time.Hour), 10000, incoming) // +$100 on Jan 11
		tx2 := createTx(account.ID, anchorDate.Add(-3*24*time.Hour), 5000, outgoing)  // -$50 on Jan 12
		tx3 := createTx(account.ID, anchorDate.Add(-2*24*time.Hour), 20000, incoming) // +$200 on Jan 13
		tx4 := createTx(account.ID, anchorDate.Add(-1*24*time.Hour), 7500, outgoing)  // -$75 on Jan 14

		err = tdb.SyncAccountBalances(ctx, account.ID)
		if err != nil {
			t.Fatalf("SyncAccountBalances failed: %v", err)
		}

		// Working backward from anchor ($1000):
		// Balance after Jan 14: 1000 (nothing between Jan 14 and anchor)
		// Balance after Jan 13: 1000 - (-75) = 1075 (undo the Jan 14 withdrawal)
		// Balance after Jan 12: 1075 - (+200) = 875 (undo the Jan 13 deposit)
		// Balance after Jan 11: 875 - (-50) = 925 (undo the Jan 12 withdrawal)
		//
		// Verify chronologically: start with X
		// After Jan 11 (+100): X + 100 = 925 → X = 825
		// After Jan 12 (-50): 925 - 50 = 875 ✓
		// After Jan 13 (+200): 875 + 200 = 1075 ✓
		// After Jan 14 (-75): 1075 - 75 = 1000 ✓ (matches anchor)
		assertBalance(t, getBalance(tx4), 100000) // $1000
		assertBalance(t, getBalance(tx3), 107500) // $1075
		assertBalance(t, getBalance(tx2), 87500)  // $875
		assertBalance(t, getBalance(tx1), 92500)  // $925
	})

	t.Run("direction zero contributes nothing", func(t *testing.T) {
		userID := tdb.CreateTestUser(ctx)
		anchorDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)

		account := tdb.CreateTestAccount(ctx, sqlc.CreateAccountParams{
			OwnerID:            userID,
			Name:               "test-direction-zero",
			Bank:               "Test Bank",
			AnchorBalanceCents: 100000,
			AnchorCurrency:     "CAD",
			MainCurrency:       "CAD",
			Colors:             []string{"#1f2937", "#3b82f6", "#10b981"},
		})

		_, err := tdb.Pool().Exec(ctx, `UPDATE accounts SET anchor_date = $1 WHERE id = $2`, anchorDate, account.ID)
		if err != nil {
			t.Fatalf("failed to update anchor_date: %v", err)
		}

		// Direction 0 (unknown) should not affect balance
		tx1 := createTx(account.ID, anchorDate.Add(24*time.Hour), 50000, 0)        // direction=0, $500
		tx2 := createTx(account.ID, anchorDate.Add(48*time.Hour), 10000, incoming) // +$100

		err = tdb.SyncAccountBalances(ctx, account.ID)
		if err != nil {
			t.Fatalf("SyncAccountBalances failed: %v", err)
		}

		// tx1 with direction=0 contributes 0, so balance stays at 1000
		// tx2 adds 100, so balance becomes 1100
		assertBalance(t, getBalance(tx1), 100000) // $1000 (unchanged)
		assertBalance(t, getBalance(tx2), 110000) // $1100
	})

	t.Run("resync after transaction update", func(t *testing.T) {
		userID := tdb.CreateTestUser(ctx)
		anchorDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)

		account := tdb.CreateTestAccount(ctx, sqlc.CreateAccountParams{
			OwnerID:            userID,
			Name:               "test-resync",
			Bank:               "Test Bank",
			AnchorBalanceCents: 100000,
			AnchorCurrency:     "CAD",
			MainCurrency:       "CAD",
			Colors:             []string{"#1f2937", "#3b82f6", "#10b981"},
		})

		_, err := tdb.Pool().Exec(ctx, `UPDATE accounts SET anchor_date = $1 WHERE id = $2`, anchorDate, account.ID)
		if err != nil {
			t.Fatalf("failed to update anchor_date: %v", err)
		}

		tx1 := createTx(account.ID, anchorDate.Add(24*time.Hour), 10000, incoming)

		err = tdb.SyncAccountBalances(ctx, account.ID)
		if err != nil {
			t.Fatalf("SyncAccountBalances failed: %v", err)
		}

		assertBalance(t, getBalance(tx1), 110000) // $1100

		// Update the transaction amount
		_, err = tdb.Pool().Exec(ctx, `UPDATE transactions SET tx_amount_cents = 30000 WHERE id = $1`, tx1)
		if err != nil {
			t.Fatalf("failed to update transaction: %v", err)
		}

		// Resync
		err = tdb.SyncAccountBalances(ctx, account.ID)
		if err != nil {
			t.Fatalf("SyncAccountBalances failed on resync: %v", err)
		}

		// Now should be 1000 + 300 = 1300
		assertBalance(t, getBalance(tx1), 130000)
	})
}

func assertBalance(t *testing.T, got *int64, want int64) {
	t.Helper()
	if got == nil {
		t.Errorf("expected balance %d, got nil", want)
		return
	}
	if *got != want {
		t.Errorf("expected balance %d, got %d", want, *got)
	}
}
