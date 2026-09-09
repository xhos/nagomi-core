package service

import (
	"testing"
	"time"

	"nagomi-core/internal/db/sqlc"
	pb "nagomi-core/internal/gen/nagomi/v1"

	"github.com/google/uuid"
	"google.golang.org/genproto/googleapis/type/money"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestBuildListTxParams_MapsAllSupportedFilters(t *testing.T) {
	userID := uuid.New()
	limit := int32(25)
	cursorID := int64(99)
	direction := pb.TransactionDirection_DIRECTION_OUTGOING
	accountID := int64(101)
	uncategorized := true
	merchantQ := "coffee"
	descQ := "latte"
	currency := "USD"

	cursorDate := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	req := &pb.ListTransactionsRequest{
		Limit:            &limit,
		Cursor:           &pb.Cursor{Date: timestamppb.New(cursorDate), Id: &cursorID},
		StartDate:        timestamppb.New(start),
		EndDate:          timestamppb.New(end),
		AmountMin:        &money.Money{CurrencyCode: "USD", Units: 12, Nanos: 340000000},
		AmountMax:        &money.Money{CurrencyCode: "USD", Units: 56, Nanos: 780000000},
		Direction:        &direction,
		AccountId:        &accountID,
		AccountIds:       []int64{202, 303},
		Categories:       []string{"food", "coffee"},
		MerchantQuery:    &merchantQ,
		DescriptionQuery: &descQ,
		Currency:         &currency,
		TimeOfDayStart:   &pb.TimeOfDay{Hours: 8, Minutes: 15},
		TimeOfDayEnd:     &pb.TimeOfDay{Hours: 18, Minutes: 45},
		Uncategorized:    &uncategorized,
	}

	params := buildListTxParams(userID, req)

	if params.UserID != userID {
		t.Fatalf("expected userID %v, got %v", userID, params.UserID)
	}
	if params.Limit == nil || *params.Limit != limit {
		t.Fatalf("expected limit %d, got %v", limit, params.Limit)
	}
	if params.CursorDate == nil || !params.CursorDate.Equal(cursorDate) {
		t.Fatalf("expected cursor date %v, got %v", cursorDate, params.CursorDate)
	}
	if params.CursorID == nil || *params.CursorID != cursorID {
		t.Fatalf("expected cursor id %d, got %v", cursorID, params.CursorID)
	}
	if params.Start == nil || !params.Start.Equal(start) {
		t.Fatalf("expected start %v, got %v", start, params.Start)
	}
	if params.End == nil || !params.End.Equal(end) {
		t.Fatalf("expected end %v, got %v", end, params.End)
	}
	if params.AmountMinCents == nil || *params.AmountMinCents != 1234 {
		t.Fatalf("expected amount min 1234, got %v", params.AmountMinCents)
	}
	if params.AmountMaxCents == nil || *params.AmountMaxCents != 5678 {
		t.Fatalf("expected amount max 5678, got %v", params.AmountMaxCents)
	}
	if params.Direction == nil || *params.Direction != int16(pb.TransactionDirection_DIRECTION_OUTGOING) {
		t.Fatalf("expected direction %d, got %v", pb.TransactionDirection_DIRECTION_OUTGOING, params.Direction)
	}
	if len(params.AccountIds) != 3 || params.AccountIds[0] != 101 || params.AccountIds[1] != 202 || params.AccountIds[2] != 303 {
		t.Fatalf("expected account ids [101 202 303], got %v", params.AccountIds)
	}
	if len(params.Categories) != 2 {
		t.Fatalf("expected categories length 2, got %d", len(params.Categories))
	}
	if params.MerchantQ == nil || *params.MerchantQ != merchantQ {
		t.Fatalf("expected merchant query %q, got %v", merchantQ, params.MerchantQ)
	}
	if params.DescQ == nil || *params.DescQ != descQ {
		t.Fatalf("expected description query %q, got %v", descQ, params.DescQ)
	}
	if params.Currency == nil || *params.Currency != currency {
		t.Fatalf("expected currency %q, got %v", currency, params.Currency)
	}
	if !params.TodStart.Valid || params.TodStart.Microseconds != int64((8*3600+15*60))*1_000_000 {
		t.Fatalf("expected tod_start valid with correct microseconds, got %+v", params.TodStart)
	}
	if !params.TodEnd.Valid || params.TodEnd.Microseconds != int64((18*3600+45*60))*1_000_000 {
		t.Fatalf("expected tod_end valid with correct microseconds, got %+v", params.TodEnd)
	}
	if params.Uncategorized == nil || *params.Uncategorized != uncategorized {
		t.Fatalf("expected uncategorized true, got %v", params.Uncategorized)
	}
}

func TestBuildListTxParams_IgnoresUnspecifiedDirection(t *testing.T) {
	userID := uuid.New()
	direction := pb.TransactionDirection_DIRECTION_UNSPECIFIED

	params := buildListTxParams(userID, &pb.ListTransactionsRequest{Direction: &direction})

	if params.Direction != nil {
		t.Fatalf("expected nil direction for unspecified enum, got %v", *params.Direction)
	}
}

func TestBuildListTxParams_IgnoresPartialCursor(t *testing.T) {
	userID := uuid.New()
	cursorDate := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)

	withDateOnly := buildListTxParams(userID, &pb.ListTransactionsRequest{
		Cursor: &pb.Cursor{Date: timestamppb.New(cursorDate)},
	})
	if withDateOnly.CursorDate != nil || withDateOnly.CursorID != nil {
		t.Fatalf("expected partial cursor to be ignored, got date=%v id=%v", withDateOnly.CursorDate, withDateOnly.CursorID)
	}

	id := int64(11)
	withIDOnly := buildListTxParams(userID, &pb.ListTransactionsRequest{
		Cursor: &pb.Cursor{Id: &id},
	})
	if withIDOnly.CursorDate != nil || withIDOnly.CursorID != nil {
		t.Fatalf("expected partial cursor to be ignored, got date=%v id=%v", withIDOnly.CursorDate, withIDOnly.CursorID)
	}
}

func TestBuildUpdateTxParams_IgnoresUnspecifiedDirection(t *testing.T) {
	userID := uuid.New()
	direction := pb.TransactionDirection_DIRECTION_UNSPECIFIED

	params := buildUpdateTxParams(userID, &pb.UpdateTransactionRequest{Id: 1, Direction: &direction})

	if params.TxDirection != nil {
		t.Fatalf("expected nil tx_direction for unspecified enum, got %v", *params.TxDirection)
	}
}

func TestToPgTimeOfDay_InvalidInputReturnsNull(t *testing.T) {
	invalidHour := toPgTimeOfDay(&pb.TimeOfDay{Hours: 24, Minutes: 0})
	if invalidHour.Valid {
		t.Fatalf("expected invalid hour to produce null pg time, got %+v", invalidHour)
	}

	invalidMinute := toPgTimeOfDay(&pb.TimeOfDay{Hours: 12, Minutes: 60})
	if invalidMinute.Valid {
		t.Fatalf("expected invalid minute to produce null pg time, got %+v", invalidMinute)
	}
}

func TestValidateCreateParams_RequiresCoreFields(t *testing.T) {
	err := validateCreateParams(sqlc.CreateTransactionParams{})
	if err == nil {
		t.Fatalf("expected error for missing required fields")
	}

	valid := sqlc.CreateTransactionParams{
		AccountID:     1,
		TxDate:        time.Now().UTC(),
		TxAmountCents: 100,
		TxCurrency:    "USD",
		TxDirection:   1,
	}
	if err := validateCreateParams(valid); err != nil {
		t.Fatalf("expected valid params, got error: %v", err)
	}
}

func TestValidateUpdateRequest_RejectsInvalidMoneyCurrency(t *testing.T) {
	err := validateUpdateRequest(&pb.UpdateTransactionRequest{
		Id:       1,
		TxAmount: &money.Money{Units: 10},
	})
	if err == nil {
		t.Fatalf("expected error when tx_amount has empty currency")
	}

	err = validateUpdateRequest(&pb.UpdateTransactionRequest{
		Id:            1,
		ForeignAmount: &money.Money{Units: 5},
	})
	if err == nil {
		t.Fatalf("expected error when foreign_amount has empty currency")
	}

	err = validateUpdateRequest(&pb.UpdateTransactionRequest{
		Id:       1,
		TxAmount: &money.Money{CurrencyCode: "USD", Units: 10},
	})
	if err != nil {
		t.Fatalf("expected valid update request, got error: %v", err)
	}
}

func TestValidateUpdateRequest_RejectsInvalidTimestamp(t *testing.T) {
	err := validateUpdateRequest(&pb.UpdateTransactionRequest{
		Id: 1,
		TxDate: &timestamppb.Timestamp{
			Seconds: 1,
			Nanos:   1_000_000_000,
		},
	})
	if err == nil {
		t.Fatalf("expected error when tx_date is invalid")
	}
}

func TestBuildCreateTxParamsList_MapsAndDefaults(t *testing.T) {
	userID := uuid.New()
	txDate := time.Date(2026, 3, 24, 12, 0, 0, 0, time.UTC)
	exchangeRate := 1.25
	merchant := "Coffee Shop"
	description := "Latte"
	categoryID := int64(7)

	req := &pb.CreateTransactionRequest{
		Transactions: []*pb.TransactionInput{
			{
				AccountId:   10,
				TxDate:      timestamppb.New(txDate),
				TxAmount:    &money.Money{CurrencyCode: "USD", Units: 12, Nanos: 340000000},
				Direction:   pb.TransactionDirection_DIRECTION_OUTGOING,
				Description: &description,
				Merchant:    &merchant,
				CategoryId:  &categoryID,
				ForeignAmount: &money.Money{
					CurrencyCode: "",
					Units:        9,
					Nanos:        990000000,
				},
				ExchangeRate: &exchangeRate,
			},
		},
	}

	paramsList := buildCreateTxParamsList(userID, req)
	if len(paramsList) != 1 {
		t.Fatalf("expected one create param, got %d", len(paramsList))
	}

	params := paramsList[0]
	if params.UserID != userID {
		t.Fatalf("expected user_id %v, got %v", userID, params.UserID)
	}
	if params.AccountID != 10 {
		t.Fatalf("expected account_id 10, got %d", params.AccountID)
	}
	if !params.TxDate.Equal(txDate) {
		t.Fatalf("expected tx_date %v, got %v", txDate, params.TxDate)
	}
	if params.TxAmountCents != 1234 {
		t.Fatalf("expected tx_amount_cents 1234, got %d", params.TxAmountCents)
	}
	if params.TxCurrency != "USD" {
		t.Fatalf("expected tx_currency USD, got %q", params.TxCurrency)
	}
	if params.TxDirection != int16(pb.TransactionDirection_DIRECTION_OUTGOING) {
		t.Fatalf("expected tx_direction outgoing, got %d", params.TxDirection)
	}
	if params.MerchantManuallySet == nil || !*params.MerchantManuallySet {
		t.Fatalf("expected merchant_manually_set=true")
	}
	if params.CategoryManuallySet == nil || !*params.CategoryManuallySet {
		t.Fatalf("expected category_manually_set=true")
	}
	if params.ForeignAmountCents == nil || *params.ForeignAmountCents != 999 {
		t.Fatalf("expected foreign_amount_cents=999, got %v", params.ForeignAmountCents)
	}
	if params.ForeignCurrency == nil || *params.ForeignCurrency != "USD" {
		t.Fatalf("expected foreign_currency default USD, got %v", params.ForeignCurrency)
	}
	if params.ExchangeRate == nil || *params.ExchangeRate != exchangeRate {
		t.Fatalf("expected exchange_rate=%v, got %v", exchangeRate, params.ExchangeRate)
	}
}

func TestBuildUpdateTxParams_MapsManualFlagsAndDefaults(t *testing.T) {
	userID := uuid.New()
	id := int64(42)
	emptyMerchant := ""
	clearCategory := int64(0)

	params := buildUpdateTxParams(userID, &pb.UpdateTransactionRequest{
		Id:         id,
		Merchant:   &emptyMerchant,
		CategoryId: &clearCategory,
		ForeignAmount: &money.Money{
			Units: 7,
			Nanos: 500000000,
		},
	})

	if params.ID != id || params.UserID != userID {
		t.Fatalf("expected id/user mapped, got id=%d user=%v", params.ID, params.UserID)
	}
	if params.MerchantManuallySet == nil || *params.MerchantManuallySet {
		t.Fatalf("expected merchant_manually_set=false when merchant cleared")
	}
	if params.CategoryManuallySet == nil || *params.CategoryManuallySet {
		t.Fatalf("expected category_manually_set=false when category cleared")
	}
	if params.ForeignAmountCents == nil || *params.ForeignAmountCents != 750 {
		t.Fatalf("expected foreign_amount_cents=750, got %v", params.ForeignAmountCents)
	}
	if params.ForeignCurrency == nil || *params.ForeignCurrency != "USD" {
		t.Fatalf("expected default foreign_currency=USD, got %v", params.ForeignCurrency)
	}
}

func TestTransactionToPb_MapsOptionalFields(t *testing.T) {
	now := time.Date(2026, 3, 24, 15, 30, 0, 0, time.UTC)
	externalID := "external-1"
	desc := "Dinner"
	merchant := "Restaurant"
	categoryID := int64(8)
	userNotes := "Business"
	balanceCents := int64(120050)
	balanceCurrency := "USD"
	foreignAmountCents := int64(100000)
	foreignCurrency := "EUR"
	exchangeRate := 1.2005

	tx := sqlc.Transaction{
		ID:                  99,
		AccountID:           12,
		ExternalID:          &externalID,
		TxDate:              now,
		TxAmountCents:       2500,
		TxCurrency:          "USD",
		TxDirection:         pb.TransactionDirection_DIRECTION_OUTGOING,
		TxDesc:              &desc,
		BalanceAfterCents:   &balanceCents,
		BalanceCurrency:     &balanceCurrency,
		Merchant:            &merchant,
		CategoryID:          &categoryID,
		CategoryManuallySet: true,
		MerchantManuallySet: true,
		UserNotes:           &userNotes,
		ForeignAmountCents:  &foreignAmountCents,
		ForeignCurrency:     &foreignCurrency,
		ExchangeRate:        &exchangeRate,
		CreatedAt:           now,
		UpdatedAt:           now,
	}

	pbTx := transactionToPb(&tx)
	if pbTx.Id != 99 || pbTx.AccountId != 12 {
		t.Fatalf("expected id/account mapped, got id=%d account=%d", pbTx.Id, pbTx.AccountId)
	}
	if pbTx.TxAmount == nil || pbTx.TxAmount.CurrencyCode != "USD" {
		t.Fatalf("expected tx amount currency USD, got %v", pbTx.TxAmount)
	}
	if pbTx.BalanceAfter == nil || pbTx.BalanceAfter.Units != 1200 {
		t.Fatalf("expected balance_after mapped, got %v", pbTx.BalanceAfter)
	}
	if pbTx.ForeignAmount == nil || pbTx.ForeignAmount.CurrencyCode != "EUR" {
		t.Fatalf("expected foreign amount mapped, got %v", pbTx.ForeignAmount)
	}
	if pbTx.ExchangeRate == nil || *pbTx.ExchangeRate != exchangeRate {
		t.Fatalf("expected exchange rate mapped, got %v", pbTx.ExchangeRate)
	}
}
