package db

import (
	"context"
	"testing"

	"null-core/internal/db/sqlc"

	"github.com/google/uuid"
)

func TestAccountAliases(t *testing.T) {
	tdb := SetupTestDB(t)
	ctx := context.Background()

	makeAccount := func(userID uuid.UUID, name string) sqlc.Account {
		t.Helper()
		return tdb.CreateTestAccount(ctx, sqlc.CreateAccountParams{
			OwnerID:        userID,
			Name:           name,
			Bank:           "Test Bank",
			AnchorCurrency: "CAD",
			MainCurrency:   "CAD",
			Colors:         []string{"#1f2937", "#3b82f6", "#10b981"},
		})
	}

	t.Run("add and find alias", func(t *testing.T) {
		userID := tdb.CreateTestUser(ctx)
		account := makeAccount(userID, "5546")

		if err := tdb.AddAccountAlias(ctx, sqlc.AddAccountAliasParams{
			ID:     account.ID,
			UserID: userID,
			Alias:  "old-5546",
		}); err != nil {
			t.Fatalf("AddAccountAlias: %v", err)
		}

		found, err := tdb.FindAccountByAlias(ctx, sqlc.FindAccountByAliasParams{
			UserID: userID,
			Alias:  "old-5546",
		})
		if err != nil {
			t.Fatalf("FindAccountByAlias: %v", err)
		}
		if found.Account.ID != account.ID {
			t.Errorf("got account %d, want %d", found.Account.ID, account.ID)
		}
	})

	t.Run("add alias is idempotent", func(t *testing.T) {
		userID := tdb.CreateTestUser(ctx)
		account := makeAccount(userID, "acct-idem")

		for i := 0; i < 3; i++ {
			if err := tdb.AddAccountAlias(ctx, sqlc.AddAccountAliasParams{
				ID:     account.ID,
				UserID: userID,
				Alias:  "same-alias",
			}); err != nil {
				t.Fatalf("AddAccountAlias iteration %d: %v", i, err)
			}
		}

		row, err := tdb.FindAccountByAlias(ctx, sqlc.FindAccountByAliasParams{
			UserID: userID,
			Alias:  "same-alias",
		})
		if err != nil {
			t.Fatalf("FindAccountByAlias: %v", err)
		}
		if len(row.Account.Aliases) != 1 {
			t.Errorf("got %d aliases, want 1", len(row.Account.Aliases))
		}
	})

	t.Run("remove alias", func(t *testing.T) {
		userID := tdb.CreateTestUser(ctx)
		account := makeAccount(userID, "acct-remove")

		_ = tdb.AddAccountAlias(ctx, sqlc.AddAccountAliasParams{ID: account.ID, UserID: userID, Alias: "to-remove"})

		if err := tdb.RemoveAccountAlias(ctx, sqlc.RemoveAccountAliasParams{
			ID:     account.ID,
			UserID: userID,
			Alias:  "to-remove",
		}); err != nil {
			t.Fatalf("RemoveAccountAlias: %v", err)
		}

		if _, err := tdb.FindAccountByAlias(ctx, sqlc.FindAccountByAliasParams{
			UserID: userID,
			Alias:  "to-remove",
		}); err == nil {
			t.Error("alias should not exist after removal")
		}
	})

	t.Run("set aliases replaces existing", func(t *testing.T) {
		userID := tdb.CreateTestUser(ctx)
		account := makeAccount(userID, "acct-set")

		_ = tdb.AddAccountAlias(ctx, sqlc.AddAccountAliasParams{ID: account.ID, UserID: userID, Alias: "old"})

		if err := tdb.SetAccountAliases(ctx, sqlc.SetAccountAliasesParams{
			ID:      account.ID,
			UserID:  userID,
			Aliases: []string{"new-1", "new-2"},
		}); err != nil {
			t.Fatalf("SetAccountAliases: %v", err)
		}

		if _, err := tdb.FindAccountByAlias(ctx, sqlc.FindAccountByAliasParams{UserID: userID, Alias: "old"}); err == nil {
			t.Error("old alias should not exist after set")
		}
		for _, alias := range []string{"new-1", "new-2"} {
			if _, err := tdb.FindAccountByAlias(ctx, sqlc.FindAccountByAliasParams{UserID: userID, Alias: alias}); err != nil {
				t.Errorf("alias %q should exist: %v", alias, err)
			}
		}
	})

	t.Run("aliases are isolated between users", func(t *testing.T) {
		userA := tdb.CreateTestUser(ctx)
		userB := tdb.CreateTestUser(ctx)
		account := makeAccount(userA, "acct-isolation")

		_ = tdb.AddAccountAlias(ctx, sqlc.AddAccountAliasParams{ID: account.ID, UserID: userA, Alias: "shared-number"})

		if _, err := tdb.FindAccountByAlias(ctx, sqlc.FindAccountByAliasParams{
			UserID: userB,
			Alias:  "shared-number",
		}); err == nil {
			t.Error("user B should not see user A's aliases")
		}
	})

	t.Run("find account by name", func(t *testing.T) {
		userID := tdb.CreateTestUser(ctx)
		account := makeAccount(userID, "3745")

		found, err := tdb.FindAccountByName(ctx, sqlc.FindAccountByNameParams{
			UserID: userID,
			Name:   "3745",
		})
		if err != nil {
			t.Fatalf("FindAccountByName: %v", err)
		}
		if found.Account.ID != account.ID {
			t.Errorf("got account %d, want %d", found.Account.ID, account.ID)
		}
	})

	t.Run("find account by name returns correct account when multiple exist", func(t *testing.T) {
		userID := tdb.CreateTestUser(ctx)
		account5546 := makeAccount(userID, "5546")
		_ = makeAccount(userID, "3745")

		found, err := tdb.FindAccountByName(ctx, sqlc.FindAccountByNameParams{
			UserID: userID,
			Name:   "5546",
		})
		if err != nil {
			t.Fatalf("FindAccountByName: %v", err)
		}
		if found.Account.ID != account5546.ID {
			t.Errorf("got account %d, want %d", found.Account.ID, account5546.ID)
		}
	})
}
