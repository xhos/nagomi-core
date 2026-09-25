package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"nagomi-core/internal/db"
	"nagomi-core/internal/db/sqlc"
	"nagomi-core/internal/exchange"
	pb "nagomi-core/internal/gen/nagomi/v1"

	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

func TestTransactionDashboardContract(t *testing.T) {
	tdb := db.SetupTestDB(t)
	ctx := context.Background()
	user := tdb.CreateTestUser(ctx)
	other := tdb.CreateTestUser(ctx)
	account := tdb.CreateTestAccount(ctx, sqlc.CreateAccountParams{OwnerID: user, Name: "contract", Bank: "test", AnchorCurrency: "CAD", MainCurrency: "CAD"})
	category, err := tdb.CreateCategory(ctx, sqlc.CreateCategoryParams{UserID: user, Slug: "food", Color: "#123456"})
	if err != nil {
		t.Fatal(err)
	}
	var categorized int64
	manual := false
	for i := 0; i < 4; i++ {
		p := sqlc.CreateTransactionParams{UserID: user, AccountID: account.ID, TxDate: time.Now().Add(time.Duration(i) * time.Minute), TxAmountCents: 1000, TxCurrency: "CAD", TxDirection: 2, CategoryManuallySet: &manual, MerchantManuallySet: &manual}
		if i == 0 {
			p.CategoryID = &category.ID
		}
		row, err := tdb.CreateTransaction(ctx, p)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			categorized = row.ID
		}
	}
	svc := newTxnSvc(tdb.Queries, nil, nil, nil, nil)
	limit, uncategorized := int32(1), true
	req := &pb.ListTransactionsRequest{Limit: &limit, Uncategorized: &uncategorized}
	for page := 0; page < 4; page++ {
		rows, cursor, err := svc.List(ctx, user, req)
		if err != nil {
			t.Fatal(err)
		}
		count, err := svc.Count(ctx, user, req)
		if err != nil || count != 3 {
			t.Fatalf("page %d: total=%d err=%v, want 3", page, count, err)
		}
		if page < 3 && len(rows) != 1 {
			t.Fatalf("page %d: got %d rows", page, len(rows))
		}
		if page == 3 && len(rows) != 0 {
			t.Fatal("expected empty final page")
		}
		req.Cursor = cursor
	}
	if count, err := svc.Count(ctx, other, req); err != nil || count != 0 {
		t.Fatalf("user isolation: count=%d err=%v", count, err)
	}
	// Omission preserves the category; a masked omission clears it and protects the choice from rules.
	if err := svc.Update(ctx, user, &pb.UpdateTransactionRequest{Id: categorized}); err != nil {
		t.Fatal(err)
	}
	row, err := svc.Get(ctx, user, categorized)
	if err != nil || row.CategoryId == nil {
		t.Fatalf("omission should preserve category: %v", err)
	}
	if err := svc.Update(ctx, user, &pb.UpdateTransactionRequest{Id: categorized, UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"category_id"}}}); err != nil {
		t.Fatal(err)
	}
	row, err = svc.Get(ctx, user, categorized)
	if err != nil || row.CategoryId != nil || !row.CategoryManuallySet {
		t.Fatalf("clear failed: row=%v err=%v", row, err)
	}
	if count, err := svc.Count(ctx, user, req); err != nil || count != 4 {
		t.Fatalf("after clear: count=%d err=%v", count, err)
	}
	req.Categories = []string{"food"}
	req.Uncategorized = nil
	if count, err := svc.Count(ctx, user, req); err != nil || count != 0 {
		t.Fatalf("category filter: count=%d err=%v", count, err)
	}
}

func TestReportingRates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/currencies" {
			_, _ = w.Write([]byte(`[{"iso_code":"CAD","name":"Canadian Dollar"},{"iso_code":"USD","name":"US Dollar"}]`))
			return
		}
		if r.URL.Path != "/rates" || r.URL.Query().Get("base") != "USD" || r.URL.Query().Get("quotes") != "CAD" {
			t.Errorf("wrong conversion: %s", r.URL)
		}
		_, _ = w.Write([]byte(`[{"date":"2026-09-24","base":"USD","quote":"CAD","rate":1.36}]`))
	}))
	defer server.Close()
	svc := newDashSvc(nil, exchange.NewClient(server.URL))
	rates, err := svc.GetExchangeRates(context.Background(), "CAD", []string{"CAD", "USD", "USD"})
	if err != nil || rates["CAD"] != 1 || rates["USD"] != 1.36 || len(rates) != 2 {
		t.Fatalf("rates=%v err=%v", rates, err)
	}
	rates, err = svc.GetExchangeRates(context.Background(), "CAD", []string{"USD", "XYZ"})
	if err != nil || rates["USD"] != 1.36 {
		t.Fatalf("unsupported currency must not fail the others: rates=%v err=%v", rates, err)
	}
	if _, ok := rates["XYZ"]; ok {
		t.Fatal("unsupported currency must be omitted, not become 1:1")
	}
}
