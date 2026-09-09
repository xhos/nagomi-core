package service

import (
	"errors"
	"testing"
	"time"

	"nagomi-core/internal/db/sqlc"
	pb "nagomi-core/internal/gen/nagomi/v1"

	"github.com/google/uuid"
	"google.golang.org/genproto/googleapis/type/money"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestBuildCreateAccountParams_DefaultsAndValidation(t *testing.T) {
	userID := uuid.New()
	friendlyName := "Main"

	params, err := buildCreateAccountParams(&pb.CreateAccountRequest{
		UserId:       userID.String(),
		Name:         "Checking",
		Bank:         "My Bank",
		Type:         pb.AccountType_ACCOUNT_CHEQUING,
		FriendlyName: &friendlyName,
		AnchorBalance: &money.Money{
			CurrencyCode: "USD",
			Units:        12,
			Nanos:        340000000,
		},
		MainCurrency: "USD",
	})
	if err != nil {
		t.Fatalf("expected valid create params, got error: %v", err)
	}

	if params.OwnerID != userID {
		t.Fatalf("expected owner_id %v, got %v", userID, params.OwnerID)
	}
	if len(params.Colors) != 3 {
		t.Fatalf("expected default colors length 3, got %d", len(params.Colors))
	}
	if params.AnchorBalanceCents != 1234 {
		t.Fatalf("expected anchor balance cents 1234, got %d", params.AnchorBalanceCents)
	}
	if params.AnchorCurrency != "USD" {
		t.Fatalf("expected anchor currency USD, got %q", params.AnchorCurrency)
	}

	_, err = buildCreateAccountParams(&pb.CreateAccountRequest{UserId: userID.String(), Colors: []string{"#1", "#2"}})
	if err == nil {
		t.Fatalf("expected error for invalid colors length")
	}

	_, err = buildCreateAccountParams(&pb.CreateAccountRequest{UserId: "not-a-uuid"})
	if err == nil {
		t.Fatalf("expected error for invalid user id")
	}
}

func TestBuildUpdateAccountParams_MapsProvidedFields(t *testing.T) {
	userID := uuid.New()
	accountType := pb.AccountType_ACCOUNT_SAVINGS
	anchorDate := time.Date(2026, 3, 25, 9, 0, 0, 0, time.UTC)
	name := "Updated"

	params := buildUpdateAccountParams(userID, &pb.UpdateAccountRequest{
		Id:          42,
		Name:        &name,
		AccountType: &accountType,
		AnchorDate:  timestamppb.New(anchorDate),
		AnchorBalance: &money.Money{
			CurrencyCode: "CAD",
			Units:        50,
		},
		Colors: []string{"#111111", "#222222", "#333333"},
	})

	if params.ID != 42 || params.UserID != userID {
		t.Fatalf("expected id/user mapped, got id=%d user=%v", params.ID, params.UserID)
	}
	if params.Name == nil || *params.Name != name {
		t.Fatalf("expected name mapped, got %v", params.Name)
	}
	if params.AccountType == nil || *params.AccountType != int16(accountType) {
		t.Fatalf("expected account type mapped, got %v", params.AccountType)
	}
	if params.AnchorDate == nil || !params.AnchorDate.Equal(anchorDate) {
		t.Fatalf("expected anchor date mapped, got %v", params.AnchorDate)
	}
	if params.AnchorBalanceCents == nil || *params.AnchorBalanceCents != 5000 {
		t.Fatalf("expected anchor balance cents 5000, got %v", params.AnchorBalanceCents)
	}
	if params.AnchorCurrency == nil || *params.AnchorCurrency != "CAD" {
		t.Fatalf("expected anchor currency CAD, got %v", params.AnchorCurrency)
	}
	if len(params.Colors) != 3 {
		t.Fatalf("expected 3 colors mapped, got %d", len(params.Colors))
	}
}

func TestNormalizeAliases_DedupTrimAndRejectEmpty(t *testing.T) {
	cleaned, err := normalizeAliases([]string{"  checking ", "checking", "savings"})
	if err != nil {
		t.Fatalf("expected normalized aliases, got error: %v", err)
	}
	if len(cleaned) != 2 || cleaned[0] != "checking" || cleaned[1] != "savings" {
		t.Fatalf("expected [checking savings], got %v", cleaned)
	}

	_, err = normalizeAliases([]string{"ok", "   "})
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("expected ErrValidation for empty alias, got %v", err)
	}
}

func TestBuildMergedAliases_PreservesPrimaryThenAddsSecondary(t *testing.T) {
	merged := buildMergedAliases(
		[]string{"primary-a", "primary-b", "primary-a"},
		" secondary-name ",
		[]string{"secondary-a", "primary-b", "   ", "secondary-a"},
	)

	expected := []string{"primary-a", "primary-b", "secondary-name", "secondary-a"}
	if len(merged) != len(expected) {
		t.Fatalf("expected %d aliases, got %d (%v)", len(expected), len(merged), merged)
	}
	for i := range expected {
		if merged[i] != expected[i] {
			t.Fatalf("expected merged[%d]=%q, got %q", i, expected[i], merged[i])
		}
	}
}

func TestAccountRowToPb_MapsCoreFields(t *testing.T) {
	now := time.Date(2026, 3, 25, 10, 0, 0, 0, time.UTC)
	friendlyName := "Daily"
	ownerID := uuid.New()

	row := sqlc.Account{
		ID:                 9,
		OwnerID:            ownerID,
		Name:               "Checking",
		Bank:               "Bank",
		AccountType:        pb.AccountType_ACCOUNT_CHEQUING,
		FriendlyName:       &friendlyName,
		AnchorDate:         now,
		AnchorBalanceCents: 120050,
		AnchorCurrency:     "USD",
		MainCurrency:       "USD",
		Colors:             []string{"#1", "#2", "#3"},
		Aliases:            []string{"main"},
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	account := accountRowToPb(row, 10050, "USD")
	if account.Id != 9 || account.OwnerId != ownerID.String() {
		t.Fatalf("expected id/owner mapped, got id=%d owner=%s", account.Id, account.OwnerId)
	}
	if account.AnchorBalance == nil || account.AnchorBalance.Units != 1200 {
		t.Fatalf("expected anchor balance mapped, got %v", account.AnchorBalance)
	}
	if account.Balance == nil || account.Balance.Units != 100 {
		t.Fatalf("expected balance mapped, got %v", account.Balance)
	}
	if len(account.Colors) != 3 || len(account.Aliases) != 1 {
		t.Fatalf("expected colors/aliases mapped, got colors=%v aliases=%v", account.Colors, account.Aliases)
	}
}
