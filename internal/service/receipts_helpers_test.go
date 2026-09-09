package service

import (
	"testing"
	"time"

	"nagomi-core/internal/db/sqlc"
	pb "nagomi-core/internal/gen/nagomi/v1"

	"github.com/google/uuid"
	datepb "google.golang.org/genproto/googleapis/type/date"
)

func TestBuildListReceiptsParams_MapsAllFilters(t *testing.T) {
	userID := uuid.New()
	limit := int32(25)
	offset := int32(5)
	status := pb.ReceiptStatus_RECEIPT_STATUS_PARSED
	unlinkedOnly := true

	params := buildListReceiptsParams(userID, &pb.ListReceiptsRequest{
		Limit:        &limit,
		Offset:       &offset,
		Status:       &status,
		UnlinkedOnly: &unlinkedOnly,
		StartDate:    &datepb.Date{Year: 2026, Month: 3, Day: 1},
		EndDate:      &datepb.Date{Year: 2026, Month: 3, Day: 25},
	})

	if params.UserID != userID {
		t.Fatalf("expected user id %v, got %v", userID, params.UserID)
	}
	if params.Lim == nil || *params.Lim != limit {
		t.Fatalf("expected limit %d, got %v", limit, params.Lim)
	}
	if params.Off == nil || *params.Off != offset {
		t.Fatalf("expected offset %d, got %v", offset, params.Off)
	}
	if params.Status == nil || *params.Status != int16(status) {
		t.Fatalf("expected status %d, got %v", status, params.Status)
	}
	if params.UnlinkedOnly == nil || *params.UnlinkedOnly != unlinkedOnly {
		t.Fatalf("expected unlinked_only true, got %v", params.UnlinkedOnly)
	}
	if params.StartDate == nil || params.StartDate.Year() != 2026 || params.StartDate.Month() != time.March || params.StartDate.Day() != 1 {
		t.Fatalf("expected mapped start_date, got %v", params.StartDate)
	}
	if params.EndDate == nil || params.EndDate.Year() != 2026 || params.EndDate.Month() != time.March || params.EndDate.Day() != 25 {
		t.Fatalf("expected mapped end_date, got %v", params.EndDate)
	}
}

func TestBuildUpdateReceiptParams_SetsLinkedStatusWhenTransactionProvided(t *testing.T) {
	userID := uuid.New()
	transactionID := int64(42)

	params := buildUpdateReceiptParams(userID, 10, &pb.UpdateReceiptRequest{TransactionId: &transactionID})
	if params.ID != 10 || params.UserID != userID {
		t.Fatalf("expected id/user mapped, got id=%d user=%v", params.ID, params.UserID)
	}
	if params.TransactionID == nil || *params.TransactionID != transactionID {
		t.Fatalf("expected transaction id %d, got %v", transactionID, params.TransactionID)
	}
	if params.Status == nil || *params.Status != int16(pb.ReceiptStatus_RECEIPT_STATUS_LINKED) {
		t.Fatalf("expected linked status, got %v", params.Status)
	}
}

func TestSelectAutoLinkMatch_RequiresExactlyOne(t *testing.T) {
	baseDate := time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC)
	bestDate := baseDate
	oneDayLater := baseDate.Add(24 * time.Hour)

	candidates := []sqlc.FindReceiptLinkCandidatesRow{
		{ID: 1, TxAmountCents: 1000, TxDate: oneDayLater},
		{ID: 2, TxAmountCents: 1001, TxDate: oneDayLater},
	}

	match, ok := selectAutoLinkMatch(candidates, 1000, &bestDate)
	if !ok || match.ID != 1 {
		t.Fatalf("expected candidate 1 to match, got ok=%v match=%+v", ok, match)
	}

	ambiguous := []sqlc.FindReceiptLinkCandidatesRow{
		{ID: 1, TxAmountCents: 1000, TxDate: oneDayLater},
		{ID: 2, TxAmountCents: 1000, TxDate: oneDayLater},
	}
	_, ok = selectAutoLinkMatch(ambiguous, 1000, &bestDate)
	if ok {
		t.Fatalf("expected no match when multiple exact candidates exist")
	}

	outOfWindow := []sqlc.FindReceiptLinkCandidatesRow{
		{ID: 9, TxAmountCents: 1000, TxDate: baseDate.Add(8 * 24 * time.Hour)},
	}
	_, ok = selectAutoLinkMatch(outOfWindow, 1000, &bestDate)
	if ok {
		t.Fatalf("expected no match when candidate falls outside date window")
	}
}

func TestReceiptHelpers_Primitives(t *testing.T) {
	if extToContentType(".jpg") != "image/jpeg" {
		t.Fatalf("expected jpg extension mapping")
	}
	if extToContentType(".unknown") != "application/octet-stream" {
		t.Fatalf("expected unknown extension fallback")
	}

	if receiptCurrencyOrDefault(nil) != "CAD" {
		t.Fatalf("expected CAD default currency")
	}
	usd := "USD"
	if receiptCurrencyOrDefault(&usd) != "USD" {
		t.Fatalf("expected explicit currency to be returned")
	}

	if dollarsToCents(10.235) != 1024 {
		t.Fatalf("expected rounding to nearest cent for dollarsToCents")
	}
	if absDiff(10, 4) != 6 || absDiff(4, 10) != 6 {
		t.Fatalf("expected absDiff symmetry")
	}
	if absDuration(-2*time.Hour) != 2*time.Hour {
		t.Fatalf("expected absDuration to normalize sign")
	}
}

func TestReceiptBestDate_Priority(t *testing.T) {
	imageTakenAt := time.Date(2026, 3, 25, 9, 0, 0, 0, time.UTC)
	receiptDate := time.Date(2026, 3, 24, 0, 0, 0, 0, time.UTC)
	createdAt := time.Date(2026, 3, 23, 0, 0, 0, 0, time.UTC)

	best := receiptBestDate(&imageTakenAt, &receiptDate, &createdAt)
	if best == nil || !best.Equal(imageTakenAt) {
		t.Fatalf("expected image_taken_at priority, got %v", best)
	}

	best = receiptBestDate(nil, &receiptDate, &createdAt)
	if best == nil || !best.Equal(receiptDate) {
		t.Fatalf("expected receipt_date as fallback, got %v", best)
	}

	best = receiptBestDate(nil, nil, &createdAt)
	if best == nil || !best.Equal(createdAt) {
		t.Fatalf("expected created_at fallback, got %v", best)
	}
}
