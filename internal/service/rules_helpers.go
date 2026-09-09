package service

import (
	"nagomi-core/internal/db/sqlc"
	pb "nagomi-core/internal/gen/nagomi/v1"
	"nagomi-core/internal/rules"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type ruleUpdateKey struct {
	categoryID int64
	merchant   string
}

func buildCreateRuleParams(userID uuid.UUID, ruleName string, conditions []byte, categoryID *int64, merchant *string) sqlc.CreateRuleParams {
	params := sqlc.CreateRuleParams{
		UserID:     userID,
		RuleName:   ruleName,
		Conditions: conditions,
	}
	if categoryID != nil {
		params.CategoryID = *categoryID
	}
	if merchant != nil {
		params.Merchant = *merchant
	}
	return params
}

func buildUpdateRuleParams(userID uuid.UUID, ruleID uuid.UUID, ruleName *string, conditions []byte, categoryID *int64, merchant *string) sqlc.UpdateRuleParams {
	params := sqlc.UpdateRuleParams{
		RuleID:   ruleID,
		UserID:   userID,
		RuleName: ruleName,
	}
	if len(conditions) > 0 {
		params.Conditions = conditions
	}
	if categoryID != nil {
		params.CategoryID = categoryID
	}
	if merchant != nil {
		params.Merchant = merchant
	}
	return params
}

func buildRuleUpdateKey(result *RuleMatchResult) ruleUpdateKey {
	key := ruleUpdateKey{}
	if result.CategoryID != nil {
		key.categoryID = *result.CategoryID
	}
	if result.Merchant != nil {
		key.merchant = *result.Merchant
	}
	return key
}

func buildBulkApplyRuleParams(userID uuid.UUID, key ruleUpdateKey, txIDs []int64) sqlc.BulkApplyRuleToTransactionsParams {
	return sqlc.BulkApplyRuleToTransactionsParams{
		CategoryID:     key.categoryID,
		Merchant:       key.merchant,
		TransactionIds: txIDs,
		UserID:         userID,
	}
}

func ruleToPb(r *sqlc.TransactionRule) *pb.Rule {
	var conditions *structpb.Struct
	if len(r.Conditions) > 0 {
		conditions = &structpb.Struct{}
		if err := conditions.UnmarshalJSON(r.Conditions); err != nil {
			conditions = nil
		}
	}

	isActive := false
	if r.IsActive != nil {
		isActive = *r.IsActive
	}

	timesApplied := int32(0)
	if r.TimesApplied != nil {
		timesApplied = *r.TimesApplied
	}

	rule := &pb.Rule{
		RuleId:        r.RuleID.String(),
		UserId:        r.UserID.String(),
		RuleName:      r.RuleName,
		CategoryId:    r.CategoryID,
		Merchant:      r.Merchant,
		Conditions:    conditions,
		IsActive:      isActive,
		PriorityOrder: r.PriorityOrder,
		RuleSource:    r.RuleSource,
		TimesApplied:  timesApplied,
	}

	if !r.CreatedAt.IsZero() {
		rule.CreatedAt = timestamppb.New(r.CreatedAt)
	}
	if !r.UpdatedAt.IsZero() {
		rule.UpdatedAt = timestamppb.New(r.UpdatedAt)
	}
	if r.LastAppliedAt != nil {
		rule.LastAppliedAt = timestamppb.New(*r.LastAppliedAt)
	}

	return rule
}

func evaluateRulesForTransaction(activeRules []sqlc.TransactionRule, tx *sqlc.Transaction, account *sqlc.GetAccountRow) *RuleMatchResult {
	result := &RuleMatchResult{}

	for _, rule := range activeRules {
		conditions, err := rules.ParseRuleConditions(rule.Conditions)
		if err != nil {
			continue
		}

		matches, err := rules.EvaluateRule(conditions, tx, account)
		if err != nil || !matches {
			continue
		}

		if result.CategoryID == nil && rule.CategoryID != nil {
			result.CategoryID = rule.CategoryID
		}

		if result.Merchant == nil && rule.Merchant != nil {
			result.Merchant = rule.Merchant
		}

		if result.CategoryID != nil && result.Merchant != nil {
			break
		}
	}

	return result
}
