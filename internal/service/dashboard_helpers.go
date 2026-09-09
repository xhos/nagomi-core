package service

import (
	"context"
	"fmt"
	"time"

	"nagomi-core/internal/db/sqlc"
	pb "nagomi-core/internal/gen/nagomi/v1"

	"github.com/google/uuid"
	"google.golang.org/genproto/googleapis/type/money"
)

type periodBounds struct {
	currentStart, currentEnd    time.Time
	previousStart, previousEnd  time.Time
	currentLabel, previousLabel string
}

func calculatePeriods(params CategorySpendingParams, now time.Time, loc *time.Location, earliestTxDate *time.Time) (*periodBounds, error) {
	p := &periodBounds{}

	switch params.PeriodType {
	case Period7Days:
		p.currentStart = startOfDay(now.AddDate(0, 0, -6), loc)
		p.currentEnd = endOfDay(now, loc)
		p.previousEnd = startOfDay(p.currentStart, loc).Add(-time.Nanosecond)
		p.previousStart = startOfDay(now.AddDate(0, 0, -13), loc)
		p.currentLabel = "Last 7 Days"
		p.previousLabel = "Previous 7 Days"

	case Period30Days:
		p.currentStart = startOfDay(now.AddDate(0, 0, -29), loc)
		p.currentEnd = endOfDay(now, loc)
		p.previousEnd = startOfDay(p.currentStart, loc).Add(-time.Nanosecond)
		p.previousStart = startOfDay(now.AddDate(0, 0, -59), loc)
		p.currentLabel = "Last 30 Days"
		p.previousLabel = "Previous 30 Days"

	case Period90Days:
		p.currentStart = startOfDay(now.AddDate(0, 0, -89), loc)
		p.currentEnd = endOfDay(now, loc)
		p.previousEnd = startOfDay(p.currentStart, loc).Add(-time.Nanosecond)
		p.previousStart = startOfDay(now.AddDate(0, 0, -179), loc)
		p.currentLabel = "Last 90 Days"
		p.previousLabel = "Previous 90 Days"

	case Period3Months:
		p.currentStart = startOfDay(now.AddDate(0, -3, 0), loc)
		p.currentEnd = endOfDay(now, loc)
		p.previousEnd = startOfDay(p.currentStart, loc).Add(-time.Nanosecond)
		p.previousStart = startOfDay(now.AddDate(0, -6, 0), loc)
		p.currentLabel = "Last 3 Months"
		p.previousLabel = "Previous 3 Months"

	case Period6Months:
		p.currentStart = startOfDay(now.AddDate(0, -6, 0), loc)
		p.currentEnd = endOfDay(now, loc)
		p.previousEnd = startOfDay(p.currentStart, loc).Add(-time.Nanosecond)
		p.previousStart = startOfDay(now.AddDate(0, -12, 0), loc)
		p.currentLabel = "Last 6 Months"
		p.previousLabel = "Previous 6 Months"

	case Period1Year:
		p.currentStart = startOfDay(now.AddDate(-1, 0, 0), loc)
		p.currentEnd = endOfDay(now, loc)
		p.previousEnd = startOfDay(p.currentStart, loc).Add(-time.Nanosecond)
		p.previousStart = startOfDay(now.AddDate(-2, 0, 0), loc)
		p.currentLabel = "Last Year"
		p.previousLabel = "Previous Year"

	case PeriodAllTime:
		if earliestTxDate != nil {
			p.currentStart = startOfDay(*earliestTxDate, loc)
		} else {
			p.currentStart = startOfDay(now.AddDate(-1, 0, 0), loc)
		}
		p.currentEnd = endOfDay(now, loc)
		duration := p.currentEnd.Sub(p.currentStart)
		p.previousEnd = startOfDay(p.currentStart, loc).Add(-time.Nanosecond)
		p.previousStart = p.previousEnd.Add(-duration)
		p.currentLabel = "All Time"
		p.previousLabel = "Previous Period"

	case PeriodCustom:
		if params.CustomStart == nil || params.CustomEnd == nil {
			return nil, wrapErr("DashboardService.GetCategorySpendingComparison",
				fmt.Errorf("custom period requires both start and end dates"))
		}

		p.currentStart = startOfDay(*params.CustomStart, loc)
		p.currentEnd = endOfDay(*params.CustomEnd, loc)

		if p.currentEnd.Before(p.currentStart) {
			return nil, wrapErr("DashboardService.GetCategorySpendingComparison",
				fmt.Errorf("end date must be after start date"))
		}

		duration := p.currentEnd.Sub(p.currentStart) + time.Nanosecond
		p.previousEnd = startOfDay(p.currentStart, loc).Add(-time.Nanosecond)
		p.previousStart = p.currentStart.Add(-duration)
		p.currentLabel = formatDateRange(p.currentStart, p.currentEnd)
		p.previousLabel = formatDateRange(p.previousStart, p.previousEnd)

	default:
		return nil, wrapErr("DashboardService.GetCategorySpendingComparison",
			fmt.Errorf("invalid period type"))
	}

	return p, nil
}

func (s *dashSvc) getUserPrimaryCurrency(ctx context.Context, userID uuid.UUID) string {
	currency, err := s.queries.GetUserPrimaryCurrency(ctx, userID)
	if err != nil {
		return "CAD"
	}
	return currency
}

func (s *dashSvc) getUserLocation(ctx context.Context, userID uuid.UUID) *time.Location {
	timezone, err := s.queries.GetUserTimezone(ctx, userID)
	if err != nil {
		return time.UTC
	}
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return time.UTC
	}
	return loc
}

func (s *dashSvc) centsToMoney(ctx context.Context, userID uuid.UUID, cents int64) (*money.Money, error) {
	currency := s.getUserPrimaryCurrency(ctx, userID)
	return &money.Money{
		CurrencyCode: currency,
		Units:        cents / 100,
		Nanos:        int32((cents % 100) * 10_000_000),
	}, nil
}

func buildDashboardSummaryParams(userID uuid.UUID, req *pb.GetDashboardSummaryRequest) sqlc.GetDashboardSummaryParams {
	return sqlc.GetDashboardSummaryParams{
		UserID: userID,
		Start:  dateToTime(req.StartDate),
		End:    dateToTime(req.EndDate),
	}
}

func buildTopCategoriesParams(userID uuid.UUID, req *pb.GetTopCategoriesRequest) sqlc.GetTopCategoriesParams {
	return sqlc.GetTopCategoriesParams{
		UserID: userID,
		Start:  dateToTime(req.StartDate),
		End:    dateToTime(req.EndDate),
		Limit:  req.Limit,
	}
}

func buildTopMerchantsParams(userID uuid.UUID, req *pb.GetTopMerchantsRequest) sqlc.GetTopMerchantsParams {
	return sqlc.GetTopMerchantsParams{
		UserID: userID,
		Start:  dateToTime(req.StartDate),
		End:    dateToTime(req.EndDate),
		Limit:  req.Limit,
	}
}

func buildMonthlyComparisonParams(userID uuid.UUID, monthsBack int32) sqlc.GetMonthlyComparisonParams {
	end := time.Now()
	start := end.AddDate(0, -int(monthsBack), 0)

	return sqlc.GetMonthlyComparisonParams{
		UserID: userID,
		Start:  &start,
		End:    &end,
	}
}

func trendPointToPb(trend *sqlc.GetDashboardTrendsRow) *pb.TrendPoint {
	if trend == nil {
		return nil
	}

	trendDate, _ := time.Parse("2006-01-02", trend.Date)
	return &pb.TrendPoint{
		Date:     timeToDate(trendDate),
		Income:   centsToMoney(trend.IncomeCents, "CAD"),
		Expenses: centsToMoney(trend.ExpenseCents, "CAD"),
	}
}

func monthlyComparisonToPb(comp *sqlc.GetMonthlyComparisonRow) *pb.MonthlyComparison {
	if comp == nil {
		return nil
	}

	return &pb.MonthlyComparison{
		Month:    comp.Month,
		Income:   centsToMoney(comp.IncomeCents, "CAD"),
		Expenses: centsToMoney(comp.ExpenseCents, "CAD"),
		Net:      centsToMoney(comp.NetCents, "CAD"),
	}
}

func topCategoryToPb(cat *sqlc.GetTopCategoriesRow) *pb.TopCategory {
	if cat == nil {
		return nil
	}

	return &pb.TopCategory{
		Slug:             cat.Slug,
		Color:            cat.Color,
		TransactionCount: cat.TransactionCount,
		TotalAmount:      centsToMoney(cat.TotalAmountCents, "CAD"),
	}
}

func topMerchantToPb(merchant *sqlc.GetTopMerchantsRow) *pb.TopMerchant {
	if merchant == nil {
		return nil
	}

	merchantName := ""
	if merchant.Merchant != nil {
		merchantName = *merchant.Merchant
	}

	return &pb.TopMerchant{
		Merchant:         merchantName,
		TransactionCount: merchant.TransactionCount,
		TotalAmount:      centsToMoney(merchant.TotalAmountCents, "CAD"),
		AvgAmount:        centsToMoney(merchant.AvgAmountCents, "CAD"),
	}
}

func accountBalanceToPb(row *sqlc.GetAccountBalancesRow) *pb.AccountBalance {
	if row == nil {
		return nil
	}

	return &pb.AccountBalance{
		Id:             row.ID,
		Name:           row.Name,
		AccountType:    pb.AccountType(row.AccountType),
		CurrentBalance: centsToMoney(row.BalanceCents, row.Currency),
		Currency:       row.Currency,
	}
}

func dashboardSummaryToPb(summary *sqlc.GetDashboardSummaryRow) *pb.DashboardSummary {
	if summary == nil {
		return nil
	}

	return &pb.DashboardSummary{
		TotalAccounts:             summary.TotalAccounts,
		TotalTransactions:         summary.TotalTransactions,
		TotalIncome:               centsToMoney(summary.TotalIncomeCents, "CAD"),
		TotalExpenses:             centsToMoney(summary.TotalExpenseCents, "CAD"),
		UncategorizedTransactions: summary.UncategorizedTransactions,
	}
}

func netWorthPointToPb(row *sqlc.GetNetWorthHistoryRow, currency string) *pb.NetWorthPoint {
	if row == nil {
		return nil
	}

	pointDate, _ := time.Parse("2006-01-02", row.Date)
	return &pb.NetWorthPoint{
		Date:     timeToDate(pointDate),
		NetWorth: centsToMoney(row.NetWorthCents, currency),
	}
}

func startOfDay(t time.Time, loc *time.Location) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
}

func endOfDay(t time.Time, loc *time.Location) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 999999999, loc)
}

func formatDateRange(start, end time.Time) string {
	if start.Year() == end.Year() && start.Month() == end.Month() {
		return fmt.Sprintf("%s %d-%d, %d",
			start.Month().String()[:3], start.Day(), end.Day(), start.Year())
	}
	return fmt.Sprintf("%s %d, %d - %s %d, %d",
		start.Month().String()[:3], start.Day(), start.Year(),
		end.Month().String()[:3], end.Day(), end.Year())
}
