package service

import (
	"reflect"
	"testing"
	"time"

	pb "nagomi-core/internal/gen/nagomi/v1"
)

func TestPlanCoverage(t *testing.T) {
	day := func(m time.Month, d int) time.Time { return time.Date(2026, m, d, 0, 0, 0, 0, time.UTC) }
	dayPtr := func(m time.Month, d int) *time.Time { t := day(m, d); return &t }
	stmt := func(id int64, start, end time.Time) coverageStatement {
		return coverageStatement{ID: id, Start: start, End: end}
	}
	imported := func(id int64, start, end time.Time) coveragePeriod {
		return coveragePeriod{Start: start, End: end, Status: pb.StatementCoverageStatus_STATEMENT_COVERAGE_STATUS_IMPORTED, StatementID: &id}
	}
	missing := func(start, end time.Time) coveragePeriod {
		return coveragePeriod{Start: start, End: end, Status: pb.StatementCoverageStatus_STATEMENT_COVERAGE_STATUS_MISSING}
	}
	due := func(start, end time.Time) coveragePeriod {
		return coveragePeriod{Start: start, End: end, Status: pb.StatementCoverageStatus_STATEMENT_COVERAGE_STATUS_DUE}
	}
	notOK := false

	tests := []struct {
		name       string
		statements []coverageStatement
		settings   coverageSettings
		today      time.Time
		want       []coveragePeriod
	}{
		{
			name:  "nothing imported and no start",
			today: day(3, 1),
			want:  nil,
		},
		{
			name:     "nothing imported: one stretch from the start",
			settings: coverageSettings{StatementsStart: dayPtr(1, 1)},
			today:    day(3, 1),
			want:     []coveragePeriod{missing(day(1, 1), day(3, 1))},
		},
		{
			name:     "nothing imported: the stretch stops at closing",
			settings: coverageSettings{StatementsStart: dayPtr(1, 1), ClosedAt: dayPtr(2, 1)},
			today:    day(3, 1),
			want:     []coveragePeriod{missing(day(1, 1), day(2, 1))},
		},
		{
			name:       "next period isn't due on its last day",
			statements: []coverageStatement{stmt(1, day(1, 15), day(2, 14))},
			today:      day(3, 14),
			want:       []coveragePeriod{imported(1, day(1, 15), day(2, 14))},
		},
		{
			name:       "due the day after the period ends",
			statements: []coverageStatement{stmt(1, day(1, 15), day(2, 14))},
			today:      day(3, 15),
			want:       []coveragePeriod{imported(1, day(1, 15), day(2, 14)), due(day(2, 15), day(3, 14))},
		},
		{
			name:       "several periods due",
			statements: []coverageStatement{stmt(1, day(1, 15), day(2, 14))},
			today:      day(4, 20),
			want: []coveragePeriod{
				imported(1, day(1, 15), day(2, 14)),
				due(day(2, 15), day(3, 14)),
				due(day(3, 15), day(4, 14)),
			},
		},
		{
			name:       "nothing due after closing",
			statements: []coverageStatement{stmt(1, day(1, 15), day(2, 14))},
			settings:   coverageSettings{ClosedAt: dayPtr(3, 1)},
			today:      day(6, 1),
			want:       []coveragePeriod{imported(1, day(1, 15), day(2, 14)), due(day(2, 15), day(3, 14))},
		},
		{
			name:       "gap between statements, a period per month",
			statements: []coverageStatement{stmt(1, day(1, 15), day(2, 14)), stmt(2, day(4, 15), day(5, 14))},
			today:      day(5, 15),
			want: []coveragePeriod{
				imported(1, day(1, 15), day(2, 14)),
				missing(day(2, 15), day(3, 14)),
				missing(day(3, 15), day(4, 14)),
				imported(2, day(4, 15), day(5, 14)),
			},
		},
		{
			name:       "gap shorter than a month at its end",
			statements: []coverageStatement{stmt(1, day(1, 15), day(2, 14)), stmt(2, day(3, 20), day(4, 19))},
			today:      day(4, 20),
			want: []coveragePeriod{
				imported(1, day(1, 15), day(2, 14)),
				missing(day(2, 15), day(3, 14)),
				missing(day(3, 15), day(3, 19)),
				imported(2, day(3, 20), day(4, 19)),
			},
		},
		{
			name:       "overlapping statements leave no gap",
			statements: []coverageStatement{stmt(1, day(1, 15), day(2, 14)), stmt(2, day(2, 1), day(2, 28))},
			today:      day(3, 1),
			want:       []coveragePeriod{imported(1, day(1, 15), day(2, 14)), imported(2, day(2, 1), day(2, 28))},
		},
		{
			name:       "missing before the first statement, back to the start",
			statements: []coverageStatement{stmt(1, day(3, 15), day(4, 14))},
			settings:   coverageSettings{StatementsStart: dayPtr(1, 15)},
			today:      day(4, 15),
			want: []coveragePeriod{
				missing(day(1, 15), day(2, 14)),
				missing(day(2, 15), day(3, 14)),
				imported(1, day(3, 15), day(4, 14)),
			},
		},
		{
			name:       "a start inside the previous period doesn't expect it",
			statements: []coverageStatement{stmt(1, day(3, 15), day(4, 14))},
			settings:   coverageSettings{StatementsStart: dayPtr(2, 20)},
			today:      day(4, 15),
			want:       []coveragePeriod{imported(1, day(3, 15), day(4, 14))},
		},
		{
			name:       "statement that doesn't add up",
			statements: []coverageStatement{{ID: 1, Start: day(1, 15), End: day(2, 14), BalanceOK: &notOK}},
			today:      day(2, 15),
			want: []coveragePeriod{{
				Start: day(1, 15), End: day(2, 14),
				Status:      pb.StatementCoverageStatus_STATEMENT_COVERAGE_STATUS_UNBALANCED,
				StatementID: func() *int64 { id := int64(1); return &id }(),
			}},
		},
		{
			name:       "month-end cycle keeps ending on the last day",
			statements: []coverageStatement{stmt(1, day(1, 1), day(1, 31))},
			today:      day(4, 5),
			want: []coveragePeriod{
				imported(1, day(1, 1), day(1, 31)),
				due(day(2, 1), day(2, 28)),
				due(day(3, 1), day(3, 31)),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := planCoverage(tt.statements, tt.settings, tt.today)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got  %+v\nwant %+v", got, tt.want)
			}
		})
	}
}
