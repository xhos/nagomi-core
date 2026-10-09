package service

import (
	"context"
	"testing"
	"time"

	"nagomi-core/internal/db"
	"nagomi-core/internal/db/sqlc"
	pb "nagomi-core/internal/gen/nagomi/v1"

	"github.com/charmbracelet/log"
)

func TestTransfersEndToEnd(t *testing.T) {
	tdb := db.SetupTestDB(t)
	ctx := context.Background()
	user := tdb.CreateTestUser(ctx)
	acct := func(name, bank string, typ pb.AccountType) int64 {
		return tdb.CreateTestAccount(ctx, sqlc.CreateAccountParams{
			OwnerID: user, Name: name, Bank: bank, AccountType: int16(typ), AnchorCurrency: "CAD", MainCurrency: "CAD",
		}).ID
	}
	savings := acct("savings", "RBC", pb.AccountType_ACCOUNT_SAVINGS)
	chequing := acct("chequing", "RBC", pb.AccountType_ACCOUNT_CHEQUING)
	wise := acct("wise cad", "Wise", pb.AccountType_ACCOUNT_CHEQUING)

	day := time.Date(2026, 10, 5, 4, 0, 0, 0, time.UTC)
	notManual := false
	create := func(account int64, cents int64, dir pb.TransactionDirection, desc string) int64 {
		row, err := tdb.CreateTransaction(ctx, sqlc.CreateTransactionParams{
			UserID: user, AccountID: account, TxDate: day, TxAmountCents: cents, TxCurrency: "CAD",
			TxDirection: int16(dir), TxDesc: &desc, CategoryManuallySet: &notManual, MerchantManuallySet: &notManual,
		})
		if err != nil {
			t.Fatal(err)
		}
		return row.ID
	}
	sweepOut := create(savings, 255000, out, "WWW TRF DDA - 8899")
	sweepIn := create(chequing, 255000, in, "Transfer WWW TRANSFER - 8899")
	rent := create(chequing, 255000, out, "Email Trfs E-TRANSFER SENT")
	topUpOut := create(chequing, 100606, out, "e-Transfer Request Fulfilled Wise Payments Ltd.")
	topUpIn := create(wise, 100000, in, "Topped up account")

	svc := newTransferSvc(tdb.Queries, log.New(nil))
	svc.Match(ctx, user, []int64{sweepOut})

	txSvc := newTxnSvc(tdb.Queries, nil, nil, nil, nil, svc)
	list, _, err := txSvc.List(ctx, user, &pb.ListTransactionsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	byID := map[int64]*pb.Transaction{}
	for _, tx := range list {
		byID[tx.Id] = tx
	}
	if got := byID[sweepOut].GetTransfer().GetCounterpartId(); got != sweepIn {
		t.Errorf("sweep counterpart = %d, want %d", got, sweepIn)
	}
	if byID[rent].Transfer != nil {
		t.Error("rent was linked as a transfer")
	}
	if fee := byID[topUpIn].GetTransfer().GetFee(); moneyToCents(fee) != 606 {
		t.Errorf("top-up fee = %v, want 6.06", fee)
	}

	// transfers are neither spending nor uncategorized; only the fee is spent
	summary, err := tdb.GetDashboardSummary(ctx, sqlc.GetDashboardSummaryParams{UserID: user})
	if err != nil {
		t.Fatal(err)
	}
	if summary.TotalExpenseCents != 255000+606 || summary.TotalIncomeCents != 0 {
		t.Errorf("expense = %d income = %d, want %d and 0", summary.TotalExpenseCents, summary.TotalIncomeCents, 255000+606)
	}
	if summary.UncategorizedTransactions != 1 {
		t.Errorf("uncategorized = %d, want 1", summary.UncategorizedTransactions)
	}

	// an unlinked pair stays apart when matching runs again
	if err := svc.Unlink(ctx, user, sweepIn, nil); err != nil {
		t.Fatal(err)
	}
	svc.Match(ctx, user, []int64{sweepOut, topUpOut})
	got, err := txSvc.Get(ctx, user, sweepOut)
	if err != nil {
		t.Fatal(err)
	}
	if got.Transfer != nil {
		t.Error("rejected transfer was linked again")
	}

	// and can still be linked by hand
	if err := svc.Link(ctx, user, sweepOut, sweepIn); err != nil {
		t.Fatal(err)
	}
	if err := svc.Link(ctx, user, sweepOut, sweepIn); err == nil {
		t.Error("linked a transaction that is already in a transfer")
	}
	got, err = txSvc.Get(ctx, user, sweepIn)
	if err != nil {
		t.Fatal(err)
	}
	if got.GetTransfer().GetMethod() != pb.TransferMethod_TRANSFER_METHOD_MANUAL {
		t.Errorf("method = %v, want manual", got.GetTransfer().GetMethod())
	}

	// a pair with nothing in common is suggested, and dismissing it sticks
	purchase := create(chequing, 5000, out, "AMAZON.CA")
	gift := create(savings, 5000, in, "E-TRANSFER RECEIVED ALEX")
	svc.Match(ctx, user, []int64{purchase})
	suggestions, err := svc.ListSuggestions(ctx, user)
	if err != nil || len(suggestions) != 1 || suggestions[0].GetIncoming().GetId() != gift {
		t.Fatalf("suggestions = %v, err %v; want the purchase and the gift", suggestions, err)
	}
	if err := svc.Unlink(ctx, user, gift, &purchase); err != nil {
		t.Fatal(err)
	}
	if suggestions, _ := svc.ListSuggestions(ctx, user); len(suggestions) != 0 {
		t.Errorf("dismissed suggestion came back: %v", suggestions)
	}
}
