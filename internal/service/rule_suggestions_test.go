package service

import (
	"testing"
	"time"

	"nagomi-core/internal/db/sqlc"
)

func TestDescriptionPattern(t *testing.T) {
	cases := map[string]string{
		"BO'S NO FRILLS 7952 TORONTO ON":         "BO'S NO FRILLS",
		"PAYPAL *STARBUCKSSF 4029357733 BC":      "PAYPAL *STARBUCKSSF",
		"ROGERS ******7679 888-764-3771 ON":      "ROGERS",
		"TIM HORTONS #6976 TORONTO ON":           "TIM HORTONS",
		"Seven-Eleven TOKYO":                     "Seven-Eleven TOKYO",
		"Fee Electronic transaction 1 @ $5.00":   "Fee Electronic transaction",
		"PAYMENT - THANK YOU / PAIEMENT - MERCI": "PAYMENT - THANK YOU / PAIEMENT - MERCI",
		"7-ELEVEN 1234 TORONTO":                  "7-ELEVEN",
		"AMAZON* CM21W93Y3 VANCOUVER BC":         "AMAZON*",
		"  Payroll  Deposit OPS ":                "Payroll  Deposit OPS",
		"":                                       "",
	}
	for in, want := range cases {
		if got := descriptionPattern(in); got != want {
			t.Errorf("descriptionPattern(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSuggestRules_RanksByCoverageAndSkipsOneOffs(t *testing.T) {
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	rows := []sqlc.ListUncategorizedDescriptionsRow{
		{TxDesc: "MCDONALDS 40346 TORONTO ON", EligibleCount: 4, TotalCount: 4, LastSeen: now},
		{TxDesc: "MCDONALDS MOBILE ORDER TOKYO", EligibleCount: 2, TotalCount: 2, LastSeen: now.AddDate(0, 1, 0)},
		{TxDesc: "Seven-Eleven TOKYO", EligibleCount: 3, TotalCount: 3, LastSeen: now},
		{TxDesc: "SEVEN-ELEVEN TOKYO", EligibleCount: 2, TotalCount: 2, LastSeen: now},
		{TxDesc: "IMMIGRATION CANADA ONLINE", EligibleCount: 1, TotalCount: 1, LastSeen: now},
		{TxDesc: "MANUAL ONLY 1234", EligibleCount: 0, TotalCount: 9, LastSeen: now},
	}
	groceries := int64(7)
	categorized := []sqlc.ListCategorizedDescriptionsRow{
		{TxDesc: "SEVEN-ELEVEN TOKYO", CategoryID: groceries, TxCount: 3},
		{TxDesc: "Seven-Eleven TOKYO", CategoryID: 8, TxCount: 1},
	}

	got := suggestRules(rows, categorized, 0)
	if len(got) != 2 {
		t.Fatalf("expected 2 suggestions, got %d: %+v", len(got), got)
	}

	if got[0].Pattern != "MCDONALDS" || got[0].TransactionCount != 6 {
		t.Errorf("first suggestion = %q x%d, want MCDONALDS x6", got[0].Pattern, got[0].TransactionCount)
	}
	if len(got[0].Examples) != 2 || !got[0].LastSeen.AsTime().Equal(now.AddDate(0, 1, 0)) {
		t.Errorf("first suggestion examples/last seen wrong: %+v", got[0])
	}
	if got[0].CategoryId != nil {
		t.Errorf("expected no category hint for mcdonalds, got %d", *got[0].CategoryId)
	}

	if got[1].Pattern != "Seven-Eleven TOKYO" || got[1].TransactionCount != 5 {
		t.Errorf("second suggestion = %q x%d, want Seven-Eleven TOKYO x5", got[1].Pattern, got[1].TransactionCount)
	}
	if got[1].CategoryId == nil || *got[1].CategoryId != groceries {
		t.Errorf("expected category hint %d, got %v", groceries, got[1].CategoryId)
	}
}

func TestSuggestRules_Limit(t *testing.T) {
	rows := []sqlc.ListUncategorizedDescriptionsRow{
		{TxDesc: "AAA SHOP", EligibleCount: 5, TotalCount: 5},
		{TxDesc: "BBB SHOP", EligibleCount: 4, TotalCount: 4},
	}
	if got := suggestRules(rows, nil, 1); len(got) != 1 || got[0].Pattern != "AAA SHOP" {
		t.Fatalf("expected only AAA SHOP, got %+v", got)
	}
}

func TestSuggestRules_BroadPatternLeavesCommonChildren(t *testing.T) {
	rows := []sqlc.ListUncategorizedDescriptionsRow{
		{TxDesc: "PAYPAL *STARBUCKSSF 4029357733 BC", EligibleCount: 6, TotalCount: 6},
		{TxDesc: "PAYPAL *PROTON 4029357733 MI", EligibleCount: 5, TotalCount: 5},
		{TxDesc: "PAYPAL 4029357733", EligibleCount: 2, TotalCount: 2},
		{TxDesc: "PAYPAL *UPS 4029357733 NB", EligibleCount: 1, TotalCount: 1},
		{TxDesc: "METRO 759 TORONTO ON", EligibleCount: 4, TotalCount: 4},
		{TxDesc: "Alp*Shanghai Metro Shanghai", EligibleCount: 1, TotalCount: 1},
	}

	got := map[string]int64{}
	for _, s := range suggestRules(rows, nil, 0) {
		got[s.Pattern] = s.TransactionCount
	}
	want := map[string]int64{"PAYPAL *STARBUCKSSF": 6, "PAYPAL *PROTON": 5, "METRO": 4, "PAYPAL": 3}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for p, n := range want {
		if got[p] != n {
			t.Errorf("%s: got %d, want %d (all: %v)", p, got[p], n, got)
		}
	}
}
