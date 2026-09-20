package service

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"nagomi-core/internal/db/sqlc"
	"nagomi-core/internal/exchange"
	pb "nagomi-core/internal/gen/nagomi/v1"

	"github.com/google/uuid"
	"google.golang.org/genproto/googleapis/type/money"
)

// ----- types -------------------------------------------------------------------------------

type AccountSummary struct {
	Summary *sqlc.GetDashboardSummaryRow
	Trends  []sqlc.GetDashboardTrendsRow
}

type PeriodType int

const (
	Period7Days PeriodType = iota
	Period30Days
	Period90Days
	PeriodCustom
	Period3Months
	Period6Months
	Period1Year
	PeriodAllTime
)

type CategorySpendingParams struct {
	UserID      uuid.UUID
	PeriodType  PeriodType
	CustomStart *time.Time
	CustomEnd   *time.Time
}

type PeriodInfo struct {
	StartDate string
	EndDate   string
	Label     string
}

type CategorySpendingResult struct {
	CurrentPeriod  PeriodInfo
	PreviousPeriod PeriodInfo
	Current        []sqlc.GetTopCategoriesRow
	Previous       []sqlc.GetTopCategoriesRow
}

type NetWorthHistoryParams struct {
	UserID      uuid.UUID
	StartDate   time.Time
	EndDate     time.Time
	Granularity int32
}

// ----- interface ---------------------------------------------------------------------------

type DashboardService interface {
	Balance(ctx context.Context, userID uuid.UUID) (*money.Money, error)
	Debt(ctx context.Context, userID uuid.UUID) (*money.Money, error)
	NetBalance(ctx context.Context, userID uuid.UUID) (*money.Money, error)
	Trends(ctx context.Context, userID uuid.UUID, req *pb.GetSpendingTrendsRequest) ([]*pb.TrendPoint, error)
	Summary(ctx context.Context, userID uuid.UUID, req *pb.GetDashboardSummaryRequest) (*pb.DashboardSummary, error)
	MonthlyComparison(ctx context.Context, userID uuid.UUID, monthsBack int32) ([]*pb.MonthlyComparison, error)
	TopCategories(ctx context.Context, userID uuid.UUID, req *pb.GetTopCategoriesRequest) ([]*pb.TopCategory, error)
	TopMerchants(ctx context.Context, userID uuid.UUID, req *pb.GetTopMerchantsRequest) ([]*pb.TopMerchant, error)
	AccountBalances(ctx context.Context, userID uuid.UUID) ([]*pb.AccountBalance, error)
	GetSpendingTrends(ctx context.Context, userID uuid.UUID, startDate string, endDate string, categoryID *int64, accountID *int64) ([]*pb.TrendPoint, error)
	GetCategorySpendingComparison(ctx context.Context, params CategorySpendingParams) (*CategorySpendingResult, error)
	GetNetWorthHistory(ctx context.Context, params NetWorthHistoryParams) ([]*pb.NetWorthPoint, error)
	GetEarliestTransactionDate(ctx context.Context, userID uuid.UUID) (time.Time, error)
	GetCurrencies(ctx context.Context) ([]*pb.CurrencyInfo, error)
	GetExchangeRates(ctx context.Context, reportingCurrency string, currencies []string) (map[string]float64, error)
}

type dashSvc struct {
	queries        *sqlc.Queries
	exchangeClient *exchange.Client
}

func newDashSvc(queries *sqlc.Queries, exchangeClient *exchange.Client) DashboardService {
	return &dashSvc{queries: queries, exchangeClient: exchangeClient}
}

// ----- methods -----------------------------------------------------------------------------

func (s *dashSvc) Balance(ctx context.Context, userID uuid.UUID) (*money.Money, error) {
	balances, err := s.queries.GetAccountBalances(ctx, userID)
	if err != nil {
		return nil, wrapErr("DashboardService.Balance", err)
	}

	totalCents := int64(0)
	for _, balance := range balances {
		if balance.BalanceCents > 0 {
			totalCents += balance.BalanceCents
		}
	}

	return s.centsToMoney(ctx, userID, totalCents)
}

func (s *dashSvc) Debt(ctx context.Context, userID uuid.UUID) (*money.Money, error) {
	balances, err := s.queries.GetAccountBalances(ctx, userID)
	if err != nil {
		return nil, wrapErr("DashboardService.Debt", err)
	}

	totalCents := int64(0)
	for _, balance := range balances {
		if balance.BalanceCents < 0 {
			totalCents += -balance.BalanceCents
		}
	}

	return s.centsToMoney(ctx, userID, totalCents)
}

func (s *dashSvc) Trends(ctx context.Context, userID uuid.UUID, req *pb.GetSpendingTrendsRequest) ([]*pb.TrendPoint, error) {
	startTime := dateToTime(req.StartDate)
	endTime := dateToTime(req.EndDate)

	params := sqlc.GetDashboardTrendsParams{
		UserID: userID,
		Start:  startTime,
		End:    endTime,
	}

	trends, err := s.queries.GetDashboardTrends(ctx, params)
	if err != nil {
		return nil, wrapErr("DashboardService.Trends", err)
	}

	result := make([]*pb.TrendPoint, len(trends))
	for i, trend := range trends {
		result[i] = trendPointToPb(&trend)
	}
	return result, nil
}

func (s *dashSvc) Summary(ctx context.Context, userID uuid.UUID, req *pb.GetDashboardSummaryRequest) (*pb.DashboardSummary, error) {
	params := buildDashboardSummaryParams(userID, req)
	summary, err := s.queries.GetDashboardSummary(ctx, params)
	if err != nil {
		return nil, wrapErr("DashboardService.Summary", err)
	}
	return dashboardSummaryToPb(&summary), nil
}

func (s *dashSvc) MonthlyComparison(ctx context.Context, userID uuid.UUID, monthsBack int32) ([]*pb.MonthlyComparison, error) {
	params := buildMonthlyComparisonParams(userID, monthsBack)
	comparison, err := s.queries.GetMonthlyComparison(ctx, params)
	if err != nil {
		return nil, wrapErr("DashboardService.MonthlyComparison", err)
	}

	result := make([]*pb.MonthlyComparison, len(comparison))
	for i, comp := range comparison {
		result[i] = monthlyComparisonToPb(&comp)
	}
	return result, nil
}

func (s *dashSvc) TopCategories(ctx context.Context, userID uuid.UUID, req *pb.GetTopCategoriesRequest) ([]*pb.TopCategory, error) {
	params := buildTopCategoriesParams(userID, req)
	categories, err := s.queries.GetTopCategories(ctx, params)
	if err != nil {
		return nil, wrapErr("DashboardService.TopCategories", err)
	}

	result := make([]*pb.TopCategory, len(categories))
	for i, cat := range categories {
		result[i] = topCategoryToPb(&cat)
	}
	return result, nil
}

func (s *dashSvc) TopMerchants(ctx context.Context, userID uuid.UUID, req *pb.GetTopMerchantsRequest) ([]*pb.TopMerchant, error) {
	params := buildTopMerchantsParams(userID, req)
	merchants, err := s.queries.GetTopMerchants(ctx, params)
	if err != nil {
		return nil, wrapErr("DashboardService.TopMerchants", err)
	}

	result := make([]*pb.TopMerchant, len(merchants))
	for i, merchant := range merchants {
		result[i] = topMerchantToPb(&merchant)
	}
	return result, nil
}

func (s *dashSvc) NetBalance(ctx context.Context, userID uuid.UUID) (*money.Money, error) {
	balances, err := s.queries.GetAccountBalances(ctx, userID)
	if err != nil {
		return nil, wrapErr("DashboardService.NetBalance", err)
	}

	totalCents := int64(0)
	for _, balance := range balances {
		totalCents += balance.BalanceCents
	}

	return s.centsToMoney(ctx, userID, totalCents)
}

func (s *dashSvc) AccountBalances(ctx context.Context, userID uuid.UUID) ([]*pb.AccountBalance, error) {
	balances, err := s.queries.GetAccountBalances(ctx, userID)
	if err != nil {
		return nil, wrapErr("DashboardService.AccountBalances", err)
	}

	result := make([]*pb.AccountBalance, len(balances))
	for i, balance := range balances {
		result[i] = accountBalanceToPb(&balance)
	}
	return result, nil
}

func (s *dashSvc) GetSpendingTrends(ctx context.Context, userID uuid.UUID, startDate string, endDate string, categoryID *int64, accountID *int64) ([]*pb.TrendPoint, error) {
	parsedStart, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		return nil, wrapErr("DashboardService.GetSpendingTrends.ParseStartDate", err)
	}

	parsedEnd, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return nil, wrapErr("DashboardService.GetSpendingTrends.ParseEndDate", err)
	}

	// TODO: categoryID and accountID ignored until query supports filtering
	_ = categoryID
	_ = accountID

	req := &pb.GetSpendingTrendsRequest{
		StartDate: timeToDate(parsedStart),
		EndDate:   timeToDate(parsedEnd),
	}

	return s.Trends(ctx, userID, req)
}

func (s *dashSvc) GetCategorySpendingComparison(
	ctx context.Context,
	params CategorySpendingParams,
) (*CategorySpendingResult, error) {
	loc := s.getUserLocation(ctx, params.UserID)
	now := time.Now().In(loc)

	// Get earliest transaction date for all-time period
	var earliestTxDate *time.Time
	if params.PeriodType == PeriodAllTime {
		date, err := s.queries.GetEarliestTransactionDate(ctx, params.UserID)
		if err == nil {
			earliestTxDate = &date
		}
	}

	periods, err := calculatePeriods(params, now, loc, earliestTxDate)
	if err != nil {
		return nil, err
	}

	current, err := s.queries.GetTopCategories(ctx, sqlc.GetTopCategoriesParams{
		UserID: params.UserID,
		Start:  &periods.currentStart,
		End:    &periods.currentEnd,
		Limit:  int32Ptr(20),
	})
	if err != nil {
		return nil, wrapErr("DashboardService.GetCategorySpendingComparison.Current", err)
	}

	previous, err := s.queries.GetTopCategories(ctx, sqlc.GetTopCategoriesParams{
		UserID: params.UserID,
		Start:  &periods.previousStart,
		End:    &periods.previousEnd,
		Limit:  int32Ptr(20),
	})
	if err != nil {
		return nil, wrapErr("DashboardService.GetCategorySpendingComparison.Previous", err)
	}

	return &CategorySpendingResult{
		CurrentPeriod: PeriodInfo{
			StartDate: periods.currentStart.Format("2006-01-02"),
			EndDate:   periods.currentEnd.Format("2006-01-02"),
			Label:     periods.currentLabel,
		},
		PreviousPeriod: PeriodInfo{
			StartDate: periods.previousStart.Format("2006-01-02"),
			EndDate:   periods.previousEnd.Format("2006-01-02"),
			Label:     periods.previousLabel,
		},
		Current:  current,
		Previous: previous,
	}, nil
}

func (s *dashSvc) GetNetWorthHistory(
	ctx context.Context,
	params NetWorthHistoryParams,
) ([]*pb.NetWorthPoint, error) {
	loc := s.getUserLocation(ctx, params.UserID)

	result, err := s.queries.GetNetWorthHistory(ctx, sqlc.GetNetWorthHistoryParams{
		UserID:      params.UserID,
		StartDate:   params.StartDate.In(loc),
		EndDate:     params.EndDate.In(loc),
		Granularity: params.Granularity,
	})
	if err != nil {
		return nil, wrapErr("DashboardService.GetNetWorthHistory", err)
	}

	// Get user's primary currency
	currency := s.getUserPrimaryCurrency(ctx, params.UserID)

	protoResult := make([]*pb.NetWorthPoint, len(result))
	for i, row := range result {
		protoResult[i] = netWorthPointToPb(&row, currency)
	}

	return protoResult, nil
}

func (s *dashSvc) GetEarliestTransactionDate(ctx context.Context, userID uuid.UUID) (time.Time, error) {
	date, err := s.queries.GetEarliestTransactionDate(ctx, userID)
	if err != nil {
		return time.Time{}, wrapErr("DashboardService.GetEarliestTransactionDate", err)
	}
	return date, nil
}

func (s *dashSvc) GetCurrencies(_ context.Context) ([]*pb.CurrencyInfo, error) {
	names, err := s.exchangeClient.GetCurrencies()
	if err != nil {
		return nil, wrapErr("DashboardService.GetCurrencies", err)
	}

	result := make([]*pb.CurrencyInfo, 0, len(names))
	for code, name := range names {
		result = append(result, &pb.CurrencyInfo{Code: code, Name: name})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Code < result[j].Code })

	return result, nil
}

func (s *dashSvc) GetExchangeRates(ctx context.Context, reportingCurrency string, currencies []string) (map[string]float64, error) {
	if len(reportingCurrency) != 3 || len(currencies) > 50 {
		return nil, fmt.Errorf("invalid reporting currency or too many currencies: %w", ErrValidation)
	}
	rates := make(map[string]float64, len(currencies))
	for _, currency := range currencies {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if len(currency) != 3 {
			return nil, fmt.Errorf("invalid currency: %w", ErrValidation)
		}
		if _, ok := rates[currency]; ok {
			continue
		}
		if currency == reportingCurrency {
			rates[currency] = 1
			continue
		}
		rate, err := s.exchangeClient.GetExchangeRate(currency, reportingCurrency, nil)
		if err != nil {
			return nil, wrapErr("DashboardService.GetExchangeRates", err)
		}
		if rate <= 0 || math.IsNaN(rate) || math.IsInf(rate, 0) {
			return nil, fmt.Errorf("invalid exchange rate for %s", currency)
		}
		rates[currency] = rate
	}
	return rates, nil
}
