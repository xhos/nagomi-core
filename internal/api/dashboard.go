package api

import (
	"context"
	"fmt"
	"sort"
	"time"

	pb "nagomi-core/internal/gen/nagomi/v1"
	"nagomi-core/internal/service"

	"connectrpc.com/connect"
	"google.golang.org/genproto/googleapis/type/date"
)

func (s *Server) GetDashboardSummary(ctx context.Context, req *connect.Request[pb.GetDashboardSummaryRequest]) (*connect.Response[pb.GetDashboardSummaryResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	summary, err := s.services.Dashboard.Summary(ctx, userID, req.Msg)
	if err != nil {
		return nil, wrapErr(err)
	}

	return connect.NewResponse(&pb.GetDashboardSummaryResponse{
		Summary: summary,
	}), nil
}

func (s *Server) GetMonthlyComparison(ctx context.Context, req *connect.Request[pb.GetMonthlyComparisonRequest]) (*connect.Response[pb.GetMonthlyComparisonResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	comparison, err := s.services.Dashboard.MonthlyComparison(ctx, userID, req.Msg.MonthsBack)
	if err != nil {
		return nil, wrapErr(err)
	}

	return connect.NewResponse(&pb.GetMonthlyComparisonResponse{
		Comparisons: comparison,
	}), nil
}

func (s *Server) GetTopCategories(ctx context.Context, req *connect.Request[pb.GetTopCategoriesRequest]) (*connect.Response[pb.GetTopCategoriesResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	categories, err := s.services.Dashboard.TopCategories(ctx, userID, req.Msg)
	if err != nil {
		return nil, wrapErr(err)
	}

	return connect.NewResponse(&pb.GetTopCategoriesResponse{
		Categories: categories,
	}), nil
}

func (s *Server) GetTopMerchants(ctx context.Context, req *connect.Request[pb.GetTopMerchantsRequest]) (*connect.Response[pb.GetTopMerchantsResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	merchants, err := s.services.Dashboard.TopMerchants(ctx, userID, req.Msg)
	if err != nil {
		return nil, wrapErr(err)
	}

	return connect.NewResponse(&pb.GetTopMerchantsResponse{
		Merchants: merchants,
	}), nil
}

func (s *Server) GetSpendingTrends(ctx context.Context, req *connect.Request[pb.GetSpendingTrendsRequest]) (*connect.Response[pb.GetSpendingTrendsResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	trends, err := s.services.Dashboard.Trends(ctx, userID, req.Msg)
	if err != nil {
		return nil, wrapErr(err)
	}

	return connect.NewResponse(&pb.GetSpendingTrendsResponse{
		Trends: trends,
	}), nil
}

func (s *Server) GetFinancialSummary(ctx context.Context, req *connect.Request[pb.GetFinancialSummaryRequest]) (*connect.Response[pb.GetFinancialSummaryResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	netBalance, err := s.services.Dashboard.NetBalance(ctx, userID)
	if err != nil {
		return nil, wrapErr(err)
	}

	totalBalance, err := s.services.Dashboard.Balance(ctx, userID)
	if err != nil {
		return nil, wrapErr(err)
	}

	totalDebt, err := s.services.Dashboard.Debt(ctx, userID)
	if err != nil {
		return nil, wrapErr(err)
	}

	return connect.NewResponse(&pb.GetFinancialSummaryResponse{
		TotalBalance: totalBalance,
		TotalDebt:    totalDebt,
		NetBalance:   netBalance,
	}), nil
}

func (s *Server) GetCategorySpendingComparison(ctx context.Context, req *connect.Request[pb.GetCategorySpendingComparisonRequest]) (*connect.Response[pb.GetCategorySpendingComparisonResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	// Map proto period type to service period type
	periodType, err := mapPeriodType(req.Msg.PeriodType)
	if err != nil {
		return nil, wrapErr(err)
	}

	// Convert custom dates if provided (dateToTime from mappers.go returns *time.Time)
	customStart := dateToTime(req.Msg.CustomStartDate)
	customEnd := dateToTime(req.Msg.CustomEndDate)

	// Get spending data from service layer
	result, err := s.services.Dashboard.GetCategorySpendingComparison(ctx, service.CategorySpendingParams{
		UserID:      userID,
		PeriodType:  periodType,
		CustomStart: customStart,
		CustomEnd:   customEnd,
	})
	if err != nil {
		return nil, wrapErr(err)
	}

	// Build a map to merge current and previous period data
	type MergedSpending struct {
		CategoryID    *int64
		Slug          string
		Color         string
		CurrentCents  int64
		CurrentCount  int64
		PreviousCents int64
		PreviousCount int64
	}

	merged := make(map[string]*MergedSpending)

	// Add current period data
	for i := range result.Current {
		row := &result.Current[i]
		key := row.Slug
		merged[key] = &MergedSpending{
			CategoryID:    nil, // Not available in GetTopCategoriesRow
			Slug:          row.Slug,
			Color:         row.Color,
			CurrentCents:  row.TotalAmountCents,
			CurrentCount:  row.TransactionCount,
			PreviousCents: 0,
			PreviousCount: 0,
		}
	}

	// Merge previous period data
	for i := range result.Previous {
		row := &result.Previous[i]
		key := row.Slug
		if existing, ok := merged[key]; ok {
			existing.PreviousCents = row.TotalAmountCents
			existing.PreviousCount = row.TransactionCount
		} else {
			// Category exists in previous but not current
			merged[key] = &MergedSpending{
				CategoryID:    nil, // Not available
				Slug:          row.Slug,
				Color:         row.Color,
				CurrentCents:  0,
				CurrentCount:  0,
				PreviousCents: row.TotalAmountCents,
				PreviousCount: row.TransactionCount,
			}
		}
	}

	// Separate categorized from uncategorized and build response
	var categories []*pb.CategorySpendingItem
	var uncategorized *pb.CategorySpendingComparison
	var totalCurrentCents int64
	var totalPreviousCents int64

	for _, spending := range merged {
		currentSpending := &pb.PeriodSpending{
			Amount:           centsToMoney(spending.CurrentCents, "CAD"),
			TransactionCount: spending.CurrentCount,
		}
		previousSpending := &pb.PeriodSpending{
			Amount:           centsToMoney(spending.PreviousCents, "CAD"),
			TransactionCount: spending.PreviousCount,
		}

		comparison := &pb.CategorySpendingComparison{
			CategoryId:     spending.CategoryID,
			CurrentPeriod:  currentSpending,
			PreviousPeriod: previousSpending,
		}

		// Track totals
		totalCurrentCents += spending.CurrentCents
		totalPreviousCents += spending.PreviousCents

		if spending.CategoryID == nil {
			// Uncategorized transactions
			uncategorized = comparison
		} else {
			// Categorized transactions
			item := &pb.CategorySpendingItem{
				Category: &pb.Category{
					Id:    *spending.CategoryID,
					Slug:  spending.Slug,
					Color: spending.Color,
				},
				Spending: comparison,
			}
			categories = append(categories, item)
		}
	}

	// Sort categories by current period amount descending
	sortCategoriesByCurrentSpending(categories)

	// Build period info from service result
	currentPeriod := &pb.PeriodInfo{
		StartDate: stringToDate(result.CurrentPeriod.StartDate),
		EndDate:   stringToDate(result.CurrentPeriod.EndDate),
		Label:     result.CurrentPeriod.Label,
	}
	previousPeriod := &pb.PeriodInfo{
		StartDate: stringToDate(result.PreviousPeriod.StartDate),
		EndDate:   stringToDate(result.PreviousPeriod.EndDate),
		Label:     result.PreviousPeriod.Label,
	}

	// Build totals
	totals := &pb.CategorySpendingTotals{
		CurrentPeriodTotal:  centsToMoney(totalCurrentCents, "CAD"),
		PreviousPeriodTotal: centsToMoney(totalPreviousCents, "CAD"),
	}

	return connect.NewResponse(&pb.GetCategorySpendingComparisonResponse{
		CurrentPeriod:  currentPeriod,
		PreviousPeriod: previousPeriod,
		Categories:     categories,
		Uncategorized:  uncategorized,
		Totals:         totals,
	}), nil
}

// sortCategoriesByCurrentSpending sorts categories by current period spending descending
func sortCategoriesByCurrentSpending(categories []*pb.CategorySpendingItem) {
	sort.Slice(categories, func(i, j int) bool {
		iCents := categories[i].Spending.CurrentPeriod.Amount.Units*100 +
			int64(categories[i].Spending.CurrentPeriod.Amount.Nanos/10000000)
		jCents := categories[j].Spending.CurrentPeriod.Amount.Units*100 +
			int64(categories[j].Spending.CurrentPeriod.Amount.Nanos/10000000)
		return iCents > jCents
	})
}

// mapPeriodType converts proto PeriodType to service PeriodType
func mapPeriodType(pt pb.PeriodType) (service.PeriodType, error) {
	switch pt {
	case pb.PeriodType_PERIOD_TYPE_7_DAYS:
		return service.Period7Days, nil
	case pb.PeriodType_PERIOD_TYPE_30_DAYS:
		return service.Period30Days, nil
	case pb.PeriodType_PERIOD_TYPE_90_DAYS:
		return service.Period90Days, nil
	case pb.PeriodType_PERIOD_TYPE_CUSTOM:
		return service.PeriodCustom, nil
	case pb.PeriodType_PERIOD_TYPE_3_MONTHS:
		return service.Period3Months, nil
	case pb.PeriodType_PERIOD_TYPE_6_MONTHS:
		return service.Period6Months, nil
	case pb.PeriodType_PERIOD_TYPE_1_YEAR:
		return service.Period1Year, nil
	case pb.PeriodType_PERIOD_TYPE_ALL_TIME:
		return service.PeriodAllTime, nil
	default:
		return 0, fmt.Errorf("invalid period type: %v", pt)
	}
}

// stringToDate converts a date string (YYYY-MM-DD) to google.type.Date
func stringToDate(s string) *date.Date {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil
	}
	return &date.Date{
		Year:  int32(t.Year()),
		Month: int32(t.Month()),
		Day:   int32(t.Day()),
	}
}

func (s *Server) GetNetWorthHistory(ctx context.Context, req *connect.Request[pb.GetNetWorthHistoryRequest]) (*connect.Response[pb.GetNetWorthHistoryResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	// Convert proto dates to time.Time
	startDate := dateToTime(req.Msg.StartDate)
	endDate := dateToTime(req.Msg.EndDate)

	if startDate == nil || endDate == nil {
		return nil, wrapErr(fmt.Errorf("start_date and end_date are required"))
	}

	// Clamp start date to earliest transaction date to avoid showing fake historical data
	earliestTxDate, err := s.services.Dashboard.GetEarliestTransactionDate(ctx, userID)
	if err == nil {
		// If start date is before earliest transaction, use earliest transaction instead
		if startDate.Before(earliestTxDate) {
			startDate = &earliestTxDate
		}
	}

	// Map proto granularity to int32
	granularity := mapGranularity(req.Msg.Granularity)

	// Get history from service layer
	dataPoints, err := s.services.Dashboard.GetNetWorthHistory(ctx, service.NetWorthHistoryParams{
		UserID:      userID,
		StartDate:   *startDate,
		EndDate:     *endDate,
		Granularity: granularity,
	})
	if err != nil {
		return nil, wrapErr(err)
	}

	return connect.NewResponse(&pb.GetNetWorthHistoryResponse{
		DataPoints: dataPoints,
	}), nil
}

// mapGranularity converts proto Granularity to int32
func mapGranularity(g pb.Granularity) int32 {
	switch g {
	case pb.Granularity_GRANULARITY_DAY:
		return 1
	case pb.Granularity_GRANULARITY_WEEK:
		return 2
	case pb.Granularity_GRANULARITY_MONTH:
		return 3
	default:
		return 1 // default to day
	}
}

func (s *Server) GetCurrencies(ctx context.Context, _ *connect.Request[pb.GetCurrenciesRequest]) (*connect.Response[pb.GetCurrenciesResponse], error) {
	currencies, err := s.services.Dashboard.GetCurrencies(ctx)
	if err != nil {
		return nil, wrapErr(err)
	}

	return connect.NewResponse(&pb.GetCurrenciesResponse{
		Currencies: currencies,
	}), nil
}
