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
