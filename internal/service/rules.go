package service

import (
	"context"

	"nagomi-core/internal/db/sqlc"
	pb "nagomi-core/internal/gen/nagomi/v1"

	"github.com/charmbracelet/log"
	"github.com/google/uuid"
)

// ----- interface ---------------------------------------------------------------------------

type RuleService interface {
	Create(ctx context.Context, userID uuid.UUID, ruleName string, conditions []byte, categoryID *int64, merchant *string) (*pb.Rule, error)
	Get(ctx context.Context, userID uuid.UUID, ruleID uuid.UUID) (*pb.Rule, error)
	Update(ctx context.Context, userID uuid.UUID, ruleID uuid.UUID, ruleName *string, conditions []byte, categoryID *int64, merchant *string) error
	Delete(ctx context.Context, userID uuid.UUID, ruleID uuid.UUID) (int64, error)
	List(ctx context.Context, userID uuid.UUID) ([]*pb.Rule, error)

	ApplyToTransaction(ctx context.Context, userID uuid.UUID, tx *sqlc.Transaction, account *sqlc.GetAccountRow) (*RuleMatchResult, error)
	ApplyToExisting(ctx context.Context, userID uuid.UUID, transactionIDs []int64) (int, error)
}

type catRuleSvc struct {
	queries *sqlc.Queries
	log     *log.Logger
}

func newCatRuleSvc(queries *sqlc.Queries, logger *log.Logger) RuleService {
	return &catRuleSvc{queries: queries, log: logger}
}

// ----- methods -----------------------------------------------------------------------------

type RuleMatchResult struct {
	CategoryID *int64
	Merchant   *string
}

func (s *catRuleSvc) Create(ctx context.Context, userID uuid.UUID, ruleName string, conditions []byte, categoryID *int64, merchant *string) (*pb.Rule, error) {
	params := buildCreateRuleParams(userID, ruleName, conditions, categoryID, merchant)

	rule, err := s.queries.CreateRule(ctx, params)
	if err != nil {
		return nil, wrapErr("RuleService.Create", err)
	}

	return ruleToPb(&rule), nil
}

func (s *catRuleSvc) Get(ctx context.Context, userID uuid.UUID, ruleID uuid.UUID) (*pb.Rule, error) {
	rule, err := s.queries.GetRule(ctx, sqlc.GetRuleParams{
		RuleID: ruleID,
		UserID: userID,
	})
	if err != nil {
		return nil, wrapErr("RuleService.Get", err)
	}

	return ruleToPb(&rule), nil
}

func (s *catRuleSvc) Update(ctx context.Context, userID uuid.UUID, ruleID uuid.UUID, ruleName *string, conditions []byte, categoryID *int64, merchant *string) error {
	params := buildUpdateRuleParams(userID, ruleID, ruleName, conditions, categoryID, merchant)

	err := s.queries.UpdateRule(ctx, params)
	if err != nil {
		return wrapErr("RuleService.Update", err)
	}

	return nil
}

func (s *catRuleSvc) Delete(ctx context.Context, userID uuid.UUID, ruleID uuid.UUID) (int64, error) {
	affected, err := s.queries.DeleteRule(ctx, sqlc.DeleteRuleParams{
		RuleID: ruleID,
		UserID: userID,
	})
	if err != nil {
		return 0, wrapErr("RuleService.Delete", err)
	}

	return affected, nil
}

func (s *catRuleSvc) List(ctx context.Context, userID uuid.UUID) ([]*pb.Rule, error) {
	rows, err := s.queries.ListRules(ctx, userID)
	if err != nil {
		return nil, wrapErr("RuleService.List", err)
	}

	result := make([]*pb.Rule, len(rows))
	for i := range rows {
		result[i] = ruleToPb(&rows[i])
	}

	return result, nil
}

func (s *catRuleSvc) ApplyToTransaction(ctx context.Context, userID uuid.UUID, tx *sqlc.Transaction, account *sqlc.GetAccountRow) (*RuleMatchResult, error) {
	activeRules, err := s.queries.GetActiveRules(ctx, userID)
	if err != nil {
		return nil, wrapErr("RuleService.ApplyToTransaction", err)
	}

	return evaluateRulesForTransaction(activeRules, tx, account), nil
}

func (s *catRuleSvc) ApplyToExisting(ctx context.Context, userID uuid.UUID, transactionIDs []int64) (int, error) {
	includeManuallySet := false
	transactions, err := s.queries.GetTransactionsForRuleApplication(ctx, sqlc.GetTransactionsForRuleApplicationParams{
		UserID:             userID,
		TransactionIds:     transactionIDs,
		IncludeManuallySet: &includeManuallySet,
	})
	if err != nil {
		return 0, wrapErr("RuleService.ApplyToExisting.FetchTransactions", err)
	}

	if len(transactions) == 0 {
		return 0, nil
	}

	activeRules, err := s.queries.GetActiveRules(ctx, userID)
	if err != nil {
		return 0, wrapErr("RuleService.ApplyToExisting.FetchRules", err)
	}

	updateGroups := make(map[ruleUpdateKey][]int64)

	for _, tx := range transactions {
		account, err := s.queries.GetAccount(ctx, sqlc.GetAccountParams{
			UserID: userID,
			ID:     tx.AccountID,
		})
		if err != nil {
			s.log.Warn("failed to fetch account for rule application", "account_id", tx.AccountID, "error", err)
			continue
		}

		ruleResult := evaluateRulesForTransaction(activeRules, &tx, &account)

		noMatch := ruleResult.CategoryID == nil && ruleResult.Merchant == nil
		if noMatch {
			continue
		}

		key := buildRuleUpdateKey(ruleResult)

		updateGroups[key] = append(updateGroups[key], tx.ID)
	}

	totalUpdated := 0
	for key, txIDs := range updateGroups {
		affected, err := s.queries.BulkApplyRuleToTransactions(ctx, buildBulkApplyRuleParams(userID, key, txIDs))
		if err != nil {
			s.log.Warn("failed to bulk apply rules", "error", err)
			continue
		}

		totalUpdated += int(affected)
	}

	return totalUpdated, nil
}
