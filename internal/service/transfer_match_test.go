package service

import (
	"slices"
	"testing"
	"time"

	pb "nagomi-core/internal/gen/nagomi/v1"
)

const (
	acctChequing = 1
	acctSavings  = 2
	acctCard     = 3
	acctWiseCAD  = 4
	acctWiseJPY  = 5
)

var testTransferAccounts = map[int64]transferAccount{
	acctChequing: {ID: acctChequing, Bank: "RBC", Names: []string{"RBC chequing 3878", "05172-5163878"}},
	acctSavings:  {ID: acctSavings, Bank: "RBC", Names: []string{"RBC savings 2458", "05172-5162458"}},
	acctCard:     {ID: acctCard, Bank: "RBC", Names: []string{"RBC credit card 5546", "5546"}, CreditCard: true},
	acctWiseCAD:  {ID: acctWiseCAD, Bank: "Wise", Names: []string{"Wise CAD", "wise-150849991"}},
	acctWiseJPY:  {ID: acctWiseJPY, Bank: "Wise", Names: []string{"Wise JPY", "wise-150849992"}},
}

var transferDay0 = time.Date(2026, 5, 4, 4, 0, 0, 0, time.UTC)

func ttx(id, account int64, day int, cents int64, dir pb.TransactionDirection, text string) transferTx {
	currency := "CAD"
	if account == acctWiseJPY {
		currency = "JPY"
	}
	return transferTx{
		ID: id, AccountID: account, Date: transferDay0.AddDate(0, 0, day),
		Cents: cents, Currency: currency, Direction: dir, Text: text,
	}
}

const (
	out = pb.TransactionDirection_DIRECTION_OUTGOING
	in  = pb.TransactionDirection_DIRECTION_INCOMING
)

func withRef(t transferTx, ref string) transferTx {
	t.Ref = &ref
	return t
}

func pairs(ps []transferPair) [][2]int64 {
	out := make([][2]int64, len(ps))
	for i, p := range ps {
		out[i] = [2]int64{p.Out, p.In}
	}
	slices.SortFunc(out, func(a, b [2]int64) int { return int(a[0] - b[0]) })
	return out
}

func TestPlanTransfers(t *testing.T) {
	tests := []struct {
		name     string
		txs      []transferTx
		rejected [][2]int64
		links    [][2]int64
		suggest  [][2]int64
	}{
		{
			name: "same-day transfers of the same amount are told apart by their reference",
			txs: []transferTx{
				ttx(1, acctSavings, 0, 100000, out, "Online Transfer to Deposit Account-2569"),
				ttx(2, acctSavings, 0, 100000, out, "Online Transfer to Deposit Account-5479"),
				ttx(3, acctChequing, 0, 100000, in, "Online Banking transfer - 5479"),
				ttx(4, acctChequing, 0, 100000, in, "Online Banking transfer - 2569"),
			},
			links: [][2]int64{{1, 4}, {2, 3}},
		},
		{
			name: "a transfer to chequing is not confused with rent paid from it",
			txs: []transferTx{
				ttx(1, acctSavings, 0, 255000, out, "WWW TRF DDA - 8899"),
				ttx(2, acctChequing, 0, 255000, in, "Transfer WWW TRANSFER - 8899"),
				ttx(3, acctChequing, 0, 255000, out, "Email Trfs E-TRANSFER SENT"),
			},
			links: [][2]int64{{1, 2}},
		},
		{
			name: "a card payment posts on the card before it leaves savings",
			txs: []transferTx{
				ttx(1, acctCard, 0, 125911, in, "PAYMENT - THANK YOU / PAIEMENT - MERCI"),
				ttx(2, acctSavings, 2, 125911, out, "Online Banking transfer - 3931"),
			},
			links: [][2]int64{{2, 1}},
		},
		{
			name: "a top-up loses a fee and names the receiving bank",
			txs: []transferTx{
				ttx(1, acctWiseCAD, 0, 100000, in, "Topped up account"),
				ttx(2, acctChequing, 2, 100606, out, "e-Transfer Request Fulfilled Wise Payments Ltd. YVNFUS"),
				// an exact-amount internal transfer the same days must not steal it
				ttx(3, acctSavings, 2, 100000, out, "Online Transfer to Deposit Account-4831"),
				ttx(4, acctChequing, 2, 100000, in, "Online Banking transfer - 4831"),
			},
			links: [][2]int64{{2, 1}, {3, 4}},
		},
		{
			name: "interchangeable sides are paired instead of left ambiguous",
			txs: []transferTx{
				ttx(1, acctWiseCAD, 0, 50000, in, "Topped up account"),
				ttx(2, acctWiseCAD, 2, 50000, in, "Topped up account"),
				ttx(3, acctChequing, 2, 50546, out, "e-Transfer Request Fulfilled Wise Payments Ltd. MGWU23"),
				ttx(4, acctChequing, 2, 50546, out, "e-Transfer Request Fulfilled Wise Payments Ltd. 73LM7B"),
			},
			links: [][2]int64{{3, 2}, {4, 1}},
		},
		{
			name: "a currency conversion is linked by the importer's reference",
			txs: []transferTx{
				withRef(ttx(1, acctWiseCAD, 0, 100000, out, "Converted 1,000.00 CAD to 114,541 JPY"), "BALANCE-5104068188"),
				withRef(ttx(2, acctWiseJPY, 0, 11454100, in, "Converted 1,000.00 CAD to 114,541 JPY"), "BALANCE-5104068188"),
			},
			links: [][2]int64{{1, 2}},
		},
		{
			name: "matching amounts with nothing else in common are only suggested",
			txs: []transferTx{
				ttx(1, acctChequing, 0, 5000, out, "AMAZON.CA"),
				ttx(2, acctSavings, 1, 5000, in, "E-TRANSFER RECEIVED ALEX"),
			},
			suggest: [][2]int64{{1, 2}},
		},
		{
			name: "two equally good but different counterparts are suggested",
			txs: []transferTx{
				ttx(1, acctSavings, 0, 20000, out, "Online Banking transfer - 1111"),
				ttx(2, acctCard, 0, 20000, in, "PAYMENT - THANK YOU"),
				ttx(3, acctChequing, 0, 20000, out, "Online Banking transfer - 2222"),
			},
			suggest: [][2]int64{{1, 2}, {3, 2}},
		},
		{
			name: "a different amount with nothing in common is not a transfer",
			txs: []transferTx{
				ttx(1, acctChequing, 0, 100606, out, "GROCERY STORE"),
				ttx(2, acctWiseCAD, 0, 100000, in, "Topped up account"),
			},
		},
		{
			name: "a number every transaction carries is not a reference",
			txs: []transferTx{
				ttx(1, acctChequing, 0, 100606, out, "POS 0517 COFFEE"),
				ttx(2, acctChequing, 0, 1200, out, "POS 0517 BAKERY"),
				ttx(3, acctChequing, 0, 900, out, "POS 0517 LUNCH"),
				ttx(4, acctSavings, 0, 100000, in, "DEPOSIT 0517"),
			},
		},
		{
			name: "a pair the user rejected stays apart",
			txs: []transferTx{
				ttx(1, acctSavings, 0, 255000, out, "WWW TRF DDA - 8899"),
				ttx(2, acctChequing, 0, 255000, in, "Transfer WWW TRANSFER - 8899"),
			},
			rejected: [][2]int64{{1, 2}},
		},
		{
			name: "transfers further apart than the window are not paired",
			txs: []transferTx{
				ttx(1, acctSavings, 0, 255000, out, "WWW TRF DDA - 8899"),
				ttx(2, acctChequing, 6, 255000, in, "Transfer WWW TRANSFER - 8899"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rejected := map[[2]int64]bool{}
			for _, r := range tt.rejected {
				rejected[r] = true
			}
			plan := planTransfers(tt.txs, testTransferAccounts, nil, rejected)
			if got := pairs(plan.Links); !slices.Equal(got, tt.links) && len(got)+len(tt.links) > 0 {
				t.Errorf("links = %v, want %v", got, tt.links)
			}
			if got := pairs(plan.Suggestions); !slices.Equal(got, tt.suggest) && len(got)+len(tt.suggest) > 0 {
				t.Errorf("suggestions = %v, want %v", got, tt.suggest)
			}
		})
	}
}

func TestPlanTransfersSkipsLinked(t *testing.T) {
	txs := []transferTx{
		ttx(1, acctSavings, 0, 255000, out, "WWW TRF DDA - 8899"),
		ttx(2, acctChequing, 0, 255000, in, "Transfer WWW TRANSFER - 8899"),
	}
	plan := planTransfers(txs, testTransferAccounts, map[int64]bool{2: true}, nil)
	if len(plan.Links)+len(plan.Suggestions) != 0 {
		t.Fatalf("paired an already linked transaction: %+v", plan)
	}
}
