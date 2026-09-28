package service

import (
	"testing"

	pb "nagomi-core/internal/gen/nagomi/v1"

	"google.golang.org/genproto/googleapis/type/date"
)

func TestLineExternalIDs(t *testing.T) {
	line := func(day int32, cents int64, desc string) *pb.ParsedStatementLine {
		return &pb.ParsedStatementLine{
			Date:        &date.Date{Year: 2025, Month: 12, Day: day},
			AmountCents: cents,
			Direction:   pb.TransactionDirection_DIRECTION_OUTGOING,
			Description: desc,
		}
	}

	statement := []*pb.ParsedStatementLine{line(16, 435, "COFFEE"), line(16, 435, "COFFEE"), line(17, 120, "BUS")}
	ids := lineExternalIDs(statement)

	if ids[0] == ids[1] {
		t.Errorf("identical lines on the same day got the same id %q", ids[0])
	}

	// a later statement overlapping this one must produce the same ids for the shared lines
	overlapping := lineExternalIDs([]*pb.ParsedStatementLine{line(10, 999, "EARLIER"), line(16, 435, "COFFEE"), line(16, 435, "COFFEE"), line(17, 120, "BUS")})
	for i, want := range ids {
		if got := overlapping[i+1]; got != want {
			t.Errorf("line %d: overlapping statement id = %q, want %q", i, got, want)
		}
	}
}

func TestBalanceAddsUp(t *testing.T) {
	cents := func(v int64) *int64 { return &v }
	yes, no := true, false
	out := func(v int64) *pb.ParsedStatementLine {
		return &pb.ParsedStatementLine{AmountCents: v, Direction: pb.TransactionDirection_DIRECTION_OUTGOING}
	}
	in := func(v int64) *pb.ParsedStatementLine {
		return &pb.ParsedStatementLine{AmountCents: v, Direction: pb.TransactionDirection_DIRECTION_INCOMING}
	}

	tests := []struct {
		name             string
		opening, closing *int64
		lines            []*pb.ParsedStatementLine
		want             *bool
	}{
		{"chequing adds up", cents(575), cents(1000), []*pb.ParsedStatementLine{in(1040), out(615)}, &yes},
		{"a missed line", cents(575), cents(1000), []*pb.ParsedStatementLine{in(1040)}, &no},
		// owing is negative, so charges push it further down
		{"credit card adds up", cents(-100000), cents(-58420), []*pb.ParsedStatementLine{out(8420), in(50000)}, &yes},
		{"no opening balance", nil, cents(1000), nil, nil},
		{"no closing balance", cents(575), nil, nil, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := balanceAddsUp(&pb.ParsedStatement{
				OpeningBalanceCents: tt.opening,
				ClosingBalanceCents: tt.closing,
				Lines:               tt.lines,
			})
			switch {
			case got == nil && tt.want == nil:
			case got == nil || tt.want == nil:
				t.Errorf("got %v, want %v", got, tt.want)
			case *got != *tt.want:
				t.Errorf("got %v, want %v", *got, *tt.want)
			}
		})
	}
}
