package service

import (
	"testing"
	"time"

	"nagomi-core/internal/db/sqlc"
	pb "nagomi-core/internal/gen/nagomi/v1"

	"github.com/google/uuid"
	datepb "google.golang.org/genproto/googleapis/type/date"
)

func TestBuildDashboardParams_Mappers(t *testing.T) {
	userID := uuid.New()
	limit := int32(12)

	summary := buildDashboardSummaryParams(userID, &pb.GetDashboardSummaryRequest{
		StartDate: &datepb.Date{Year: 2026, Month: 3, Day: 1},
		EndDate:   &datepb.Date{Year: 2026, Month: 3, Day: 31},
	})
	if summary.UserID != userID {
		t.Fatalf("expected summary user_id %v, got %v", userID, summary.UserID)
	}
	if summary.Start == nil || summary.End == nil {
		t.Fatalf("expected summary start/end to be mapped")
	}

	topCats := buildTopCategoriesParams(userID, &pb.GetTopCategoriesRequest{Limit: &limit})
	if topCats.Limit == nil || *topCats.Limit != limit {
		t.Fatalf("expected top categories limit %d, got %v", limit, topCats.Limit)
	}

	topMerchants := buildTopMerchantsParams(userID, &pb.GetTopMerchantsRequest{Limit: &limit})
	if topMerchants.Limit == nil || *topMerchants.Limit != limit {
		t.Fatalf("expected top merchants limit %d, got %v", limit, topMerchants.Limit)
	}

	monthly := buildMonthlyComparisonParams(userID, 3)
	if monthly.UserID != userID || monthly.Start == nil || monthly.End == nil {
		t.Fatalf("expected monthly params to contain user/start/end, got %+v", monthly)
	}
	if !monthly.End.After(*monthly.Start) {
		t.Fatalf("expected monthly end after start, got start=%v end=%v", monthly.Start, monthly.End)
	}
}

func TestDayBoundariesAndFormatRange(t *testing.T) {
	loc := time.FixedZone("EST", -5*3600)
	tm := time.Date(2026, 3, 25, 14, 30, 10, 123, loc)

	start := startOfDay(tm, loc)
	if start.Hour() != 0 || start.Minute() != 0 || start.Second() != 0 || start.Nanosecond() != 0 {
		t.Fatalf("expected start of day, got %v", start)
	}

	end := endOfDay(tm, loc)
	if end.Hour() != 23 || end.Minute() != 59 || end.Second() != 59 || end.Nanosecond() != 999999999 {
		t.Fatalf("expected end of day, got %v", end)
	}

	sameMonth := formatDateRange(
		time.Date(2026, 3, 1, 0, 0, 0, 0, loc),
		time.Date(2026, 3, 10, 0, 0, 0, 0, loc),
	)
	if sameMonth != "Mar 1-10, 2026" {
		t.Fatalf("unexpected same-month range format: %q", sameMonth)
	}

	crossMonth := formatDateRange(
		time.Date(2026, 3, 28, 0, 0, 0, 0, loc),
		time.Date(2026, 4, 2, 0, 0, 0, 0, loc),
	)
	if crossMonth != "Mar 28, 2026 - Apr 2, 2026" {
		t.Fatalf("unexpected cross-month range format: %q", crossMonth)
	}
}

func TestCalculatePeriods_CustomValidationAndRange(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 3, 25, 10, 0, 0, 0, time.UTC)

	_, err := calculatePeriods(CategorySpendingParams{PeriodType: PeriodCustom}, now, loc, nil)
	if err == nil {
		t.Fatalf("expected error when custom dates are missing")
	}

	start := time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC)
	_, err = calculatePeriods(CategorySpendingParams{
		PeriodType:  PeriodCustom,
		CustomStart: &start,
		CustomEnd:   &end,
	}, now, loc, nil)
	if err == nil {
		t.Fatalf("expected error when custom end precedes start")
	}

	validEnd := time.Date(2026, 3, 25, 0, 0, 0, 0, time.UTC)
	periods, err := calculatePeriods(CategorySpendingParams{
		PeriodType:  PeriodCustom,
		CustomStart: &start,
		CustomEnd:   &validEnd,
	}, now, loc, nil)
	if err != nil {
		t.Fatalf("expected valid custom period, got error: %v", err)
	}
	if periods.currentLabel == "" || periods.previousLabel == "" {
		t.Fatalf("expected labels to be populated, got %+v", periods)
	}
	if !periods.currentEnd.After(periods.currentStart) {
		t.Fatalf("expected current period end after start, got %+v", periods)
	}
}

func TestDashboardConverters(t *testing.T) {
	trend := trendPointToPb(&sqlc.GetDashboardTrendsRow{Date: "2026-03-25", IncomeCents: 1050, ExpenseCents: 2300})
	if trend == nil || trend.Income == nil || trend.Expenses == nil {
		t.Fatalf("expected trend conversion with money fields, got %+v", trend)
	}

	monthly := monthlyComparisonToPb(&sqlc.GetMonthlyComparisonRow{Month: "2026-03", IncomeCents: 1000, ExpenseCents: 200, NetCents: 800})
	if monthly == nil || monthly.Net == nil || monthly.Month != "2026-03" {
		t.Fatalf("expected monthly conversion, got %+v", monthly)
	}

	merchant := "Coffee"
	tm := topMerchantToPb(&sqlc.GetTopMerchantsRow{Merchant: &merchant, TransactionCount: 2, TotalAmountCents: 900, AvgAmountCents: 450})
	if tm == nil || tm.Merchant != merchant || tm.TotalAmount == nil {
		t.Fatalf("expected top merchant conversion, got %+v", tm)
	}

	summary := dashboardSummaryToPb(&sqlc.GetDashboardSummaryRow{TotalAccounts: 3, TotalTransactions: 10, TotalIncomeCents: 5000, TotalExpenseCents: 3200})
	if summary == nil || summary.TotalAccounts != 3 || summary.TotalExpenses == nil {
		t.Fatalf("expected dashboard summary conversion, got %+v", summary)
	}

	nw := netWorthPointToPb(&sqlc.GetNetWorthHistoryRow{Date: "2026-03-25", NetWorthCents: 12345}, "USD")
	if nw == nil || nw.NetWorth == nil || nw.NetWorth.CurrencyCode != "USD" {
		t.Fatalf("expected net worth conversion with currency, got %+v", nw)
	}
}
