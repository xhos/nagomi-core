package service

import (
	"testing"
	"time"

	"nagomi-core/internal/db/sqlc"

	"github.com/google/uuid"
)

func TestBuildCreateRuleParams_MapsOptionalFields(t *testing.T) {
	userID := uuid.New()
	categoryID := int64(9)
	merchant := "Coffee Shop"
	conditions := []byte(`{"conditions":[]}`)

	withAll := buildCreateRuleParams(userID, "Rule A", conditions, &categoryID, &merchant)
	if withAll.UserID != userID || withAll.RuleName != "Rule A" {
		t.Fatalf("expected user/rule_name mapped, got %+v", withAll)
	}
	if withAll.CategoryID != categoryID {
		t.Fatalf("expected category_id %d, got %d", categoryID, withAll.CategoryID)
	}
	if withAll.Merchant != merchant {
		t.Fatalf("expected merchant %q, got %q", merchant, withAll.Merchant)
	}

	withoutOptional := buildCreateRuleParams(userID, "Rule B", conditions, nil, nil)
	if withoutOptional.CategoryID != 0 {
		t.Fatalf("expected zero category when nil, got %d", withoutOptional.CategoryID)
	}
	if withoutOptional.Merchant != "" {
		t.Fatalf("expected empty merchant when nil, got %q", withoutOptional.Merchant)
	}
}

func TestBuildUpdateRuleParams_ConditionsAndPointers(t *testing.T) {
	userID := uuid.New()
	ruleID := uuid.New()
	ruleName := "Updated"
	categoryID := int64(22)
	merchant := "Store"

	params := buildUpdateRuleParams(userID, ruleID, &ruleName, []byte(`{"conditions":[1]}`), &categoryID, &merchant)
	if params.RuleID != ruleID || params.UserID != userID {
		t.Fatalf("expected ids mapped, got %+v", params)
	}
	if params.RuleName == nil || *params.RuleName != ruleName {
		t.Fatalf("expected rule name pointer mapped, got %v", params.RuleName)
	}
	if len(params.Conditions) == 0 {
		t.Fatalf("expected conditions copied")
	}
	if params.CategoryID == nil || *params.CategoryID != categoryID {
		t.Fatalf("expected category pointer mapped, got %v", params.CategoryID)
	}
	if params.Merchant == nil || *params.Merchant != merchant {
		t.Fatalf("expected merchant pointer mapped, got %v", params.Merchant)
	}

	noConditions := buildUpdateRuleParams(userID, ruleID, nil, nil, nil, nil)
	if len(noConditions.Conditions) != 0 {
		t.Fatalf("expected empty conditions when not provided")
	}
}

func TestRuleUpdateKeyAndBulkParams(t *testing.T) {
	userID := uuid.New()
	categoryID := int64(7)
	merchant := "Cafe"
	result := &RuleMatchResult{CategoryID: &categoryID, Merchant: &merchant}

	key := buildRuleUpdateKey(result)
	if key.categoryID != categoryID || key.merchant != merchant {
		t.Fatalf("expected key fields mapped, got %+v", key)
	}

	params := buildBulkApplyRuleParams(userID, key, []int64{1, 2, 3})
	if params.UserID != userID || params.CategoryID != categoryID || params.Merchant != merchant {
		t.Fatalf("expected bulk params mapped, got %+v", params)
	}
	if len(params.TransactionIds) != 3 {
		t.Fatalf("expected tx ids mapped, got %v", params.TransactionIds)
	}
}

func TestRuleToPb_ParsesConditionsAndTimestamps(t *testing.T) {
	now := time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)
	categoryID := int64(11)
	merchant := "Cafe"
	active := true
	timesApplied := int32(5)
	lastApplied := now.Add(-time.Hour)

	rule := &sqlc.TransactionRule{
		RuleID:        uuid.New(),
		UserID:        uuid.New(),
		RuleName:      "Groceries Rule",
		CategoryID:    &categoryID,
		Merchant:      &merchant,
		Conditions:    []byte(`{"logic":"and","conditions":[]}`),
		IsActive:      &active,
		PriorityOrder: 10,
		RuleSource:    "manual",
		CreatedAt:     now,
		UpdatedAt:     now,
		LastAppliedAt: &lastApplied,
		TimesApplied:  &timesApplied,
	}

	pbRule := ruleToPb(rule)
	if pbRule.RuleId == "" || pbRule.UserId == "" {
		t.Fatalf("expected ids mapped, got %+v", pbRule)
	}
	if pbRule.Conditions == nil {
		t.Fatalf("expected conditions mapped")
	}
	if pbRule.CategoryId == nil || *pbRule.CategoryId != categoryID {
		t.Fatalf("expected category mapped, got %v", pbRule.CategoryId)
	}
	if pbRule.Merchant == nil || *pbRule.Merchant != merchant {
		t.Fatalf("expected merchant mapped, got %v", pbRule.Merchant)
	}
	if !pbRule.IsActive || pbRule.TimesApplied != timesApplied {
		t.Fatalf("expected active/times mapped, got active=%v times=%d", pbRule.IsActive, pbRule.TimesApplied)
	}
	if pbRule.CreatedAt == nil || pbRule.UpdatedAt == nil || pbRule.LastAppliedAt == nil {
		t.Fatalf("expected timestamps mapped, got %+v", pbRule)
	}
}

func TestEvaluateRulesForTransaction_InvalidConditionsNoMatch(t *testing.T) {
	account := &sqlc.GetAccountRow{}
	tx := &sqlc.Transaction{}

	rules := []sqlc.TransactionRule{
		{Conditions: []byte(`not-json`)},
	}

	result := evaluateRulesForTransaction(rules, tx, account)
	if result == nil {
		t.Fatalf("expected non-nil match result")
	}
	if result.CategoryID != nil || result.Merchant != nil {
		t.Fatalf("expected no match for invalid conditions, got %+v", result)
	}
}
