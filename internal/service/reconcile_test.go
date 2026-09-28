package service

import (
	"reflect"
	"testing"
	"time"

	pb "nagomi-core/internal/gen/nagomi/v1"
)

func TestPlanReconcile(t *testing.T) {
	day := func(d int) time.Time { return time.Date(2026, 3, d, 0, 0, 0, 0, time.UTC) }
	out := pb.TransactionDirection_DIRECTION_OUTGOING
	in := pb.TransactionDirection_DIRECTION_INCOMING
	email := pb.TransactionSource_TRANSACTION_SOURCE_EMAIL
	manual := pb.TransactionSource_TRANSACTION_SOURCE_MANUAL
	line := func(d int, cents int64) reconcileLine { return reconcileLine{Day: day(d), AmountCents: cents, Direction: out} }
	cand := func(id int64, d int, cents int64) reconcileCandidate {
		return reconcileCandidate{ID: id, Day: day(d), AmountCents: cents, Direction: out, Source: email}
	}
	periodStart, periodEnd := day(1), day(31)

	tests := []struct {
		name       string
		lines      []reconcileLine
		candidates []reconcileCandidate
		want       reconcilePlan
	}{
		{
			name:       "exact match within the date window",
			lines:      []reconcileLine{line(10, 435)},
			candidates: []reconcileCandidate{cand(1, 8, 435)},
			want:       reconcilePlan{Matches: []reconcileMatch{{Line: 0, TransactionID: 1}}},
		},
		{
			name:       "too far apart to match",
			lines:      []reconcileLine{line(10, 435)},
			candidates: []reconcileCandidate{cand(1, 6, 435)},
			want:       reconcilePlan{Create: []int{0}, Delete: []int64{1}},
		},
		{
			name:       "direction must agree",
			lines:      []reconcileLine{{Day: day(10), AmountCents: 435, Direction: in}},
			candidates: []reconcileCandidate{cand(1, 10, 435)},
			want:       reconcilePlan{Create: []int{0}, Delete: []int64{1}},
		},
		{
			name:  "two identical coffees pair by closest date",
			lines: []reconcileLine{line(10, 435), line(12, 435)},
			candidates: []reconcileCandidate{
				cand(1, 12, 435),
				cand(2, 10, 435),
			},
			want: reconcilePlan{Matches: []reconcileMatch{{Line: 0, TransactionID: 2}, {Line: 1, TransactionID: 1}}},
		},
		{
			name:       "tip changes the amount",
			lines:      []reconcileLine{line(10, 5750)},
			candidates: []reconcileCandidate{cand(1, 9, 5000)},
			want:       reconcilePlan{Matches: []reconcileMatch{{Line: 0, TransactionID: 1, AmountChanged: true}}},
		},
		{
			name:       "exact beats near",
			lines:      []reconcileLine{line(10, 5000)},
			candidates: []reconcileCandidate{cand(1, 10, 4800), cand(2, 10, 5000)},
			want:       reconcilePlan{Matches: []reconcileMatch{{Line: 0, TransactionID: 2}}, Delete: []int64{1}},
		},
		{
			name:       "ambiguous near matches are left alone",
			lines:      []reconcileLine{line(10, 5750)},
			candidates: []reconcileCandidate{cand(1, 10, 5000), cand(2, 11, 5100)},
			want:       reconcilePlan{Create: []int{0}, Delete: []int64{1, 2}},
		},
		{
			name:       "beyond the tolerance",
			lines:      []reconcileLine{line(10, 10000)},
			candidates: []reconcileCandidate{cand(1, 10, 7000)},
			want:       reconcilePlan{Create: []int{0}, Delete: []int64{1}},
		},
		{
			name:  "leftovers: manual, user data and grace window are kept",
			lines: nil,
			candidates: []reconcileCandidate{
				{ID: 1, Day: day(10), AmountCents: 100, Direction: out, Source: manual},
				{ID: 2, Day: day(10), AmountCents: 100, Direction: out, Source: email, HasUserData: true},
				cand(3, 28, 100),
				cand(4, 10, 100),
			},
			want: reconcilePlan{
				Delete: []int64{4},
				Keep: []reconcileKept{
					{1, pb.ReconciliationKeepReason_RECONCILIATION_KEEP_REASON_MANUAL},
					{2, pb.ReconciliationKeepReason_RECONCILIATION_KEEP_REASON_USER_DATA},
					{3, pb.ReconciliationKeepReason_RECONCILIATION_KEEP_REASON_GRACE},
				},
			},
		},
		{
			name:       "unmatched candidates outside the period are ignored",
			lines:      nil,
			candidates: []reconcileCandidate{{ID: 1, Day: time.Date(2026, 2, 27, 0, 0, 0, 0, time.UTC), AmountCents: 100, Direction: out, Source: email}},
			want:       reconcilePlan{},
		},
		{
			name:       "line on the period end matches a transaction just after it",
			lines:      []reconcileLine{line(31, 999)},
			candidates: []reconcileCandidate{{ID: 1, Day: time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC), AmountCents: 999, Direction: out, Source: email}},
			want:       reconcilePlan{Matches: []reconcileMatch{{Line: 0, TransactionID: 1}}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := planReconcile(tt.lines, tt.candidates, periodStart, periodEnd)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got  %+v\nwant %+v", got, tt.want)
			}
		})
	}
}
