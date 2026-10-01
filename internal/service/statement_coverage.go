package service

import (
	"time"

	pb "nagomi-core/internal/gen/nagomi/v1"
)

const (
	// without a release day, a statement is due this many days after its period ends
	statementReleaseLagDays = 3
	// bounds the periods generated around a statement, in case of a decades-old one
	maxCoveragePeriods = 600
)

// coverageStatement is an imported statement's period, as calendar days in UTC.
type coverageStatement struct {
	ID        int64
	Start     time.Time
	End       time.Time
	BalanceOK *bool
}

type coverageSettings struct {
	// periods starting before this aren't expected
	StatementsStart *time.Time
	ReleaseDay      *int16
	// periods starting after this aren't expected
	ClosedAt *time.Time
}

type coveragePeriod struct {
	Start       time.Time
	End         time.Time
	Status      pb.StatementCoverageStatus
	StatementID *int64
}

// planCoverage lays an account's statements out as consecutive periods, oldest first:
// missing ones before the first statement and between statements, then the ones due
// since the last. periods it has to guess run a month from the neighbouring statement.
// statements must be sorted by start.
func planCoverage(statements []coverageStatement, settings coverageSettings, today time.Time) []coveragePeriod {
	missing := pb.StatementCoverageStatus_STATEMENT_COVERAGE_STATUS_MISSING

	if len(statements) == 0 {
		// nothing to tell the cycle from, so one stretch from the start
		if settings.StatementsStart == nil {
			return nil
		}
		end := today
		if settings.ClosedAt != nil && settings.ClosedAt.Before(end) {
			end = *settings.ClosedAt
		}
		if settings.StatementsStart.After(end) {
			return nil
		}
		return []coveragePeriod{{Start: *settings.StatementsStart, End: end, Status: missing}}
	}

	var out []coveragePeriod

	first := statements[0].Start
	if settings.StatementsStart != nil {
		var before []coveragePeriod
		for i := 1; i <= maxCoveragePeriods; i++ {
			start := addMonthsClamped(first, -i)
			if start.Before(*settings.StatementsStart) {
				break
			}
			before = append(before, coveragePeriod{Start: start, End: addMonthsClamped(first, 1-i).AddDate(0, 0, -1), Status: missing})
		}
		for i := len(before) - 1; i >= 0; i-- {
			out = append(out, before[i])
		}
	}

	// the last day some statement covers; statements may overlap
	covered := first.AddDate(0, 0, -1)
	for _, stmt := range statements {
		if gapStart := covered.AddDate(0, 0, 1); gapStart.Before(stmt.Start) {
			out = append(out, monthlyPeriods(gapStart, stmt.Start.AddDate(0, 0, -1), missing)...)
		}

		status := pb.StatementCoverageStatus_STATEMENT_COVERAGE_STATUS_IMPORTED
		if stmt.BalanceOK != nil && !*stmt.BalanceOK {
			status = pb.StatementCoverageStatus_STATEMENT_COVERAGE_STATUS_UNBALANCED
		}
		out = append(out, coveragePeriod{Start: stmt.Start, End: stmt.End, Status: status, StatementID: &stmt.ID})

		if stmt.End.After(covered) {
			covered = stmt.End
		}
	}

	for i := 1; i <= maxCoveragePeriods; i++ {
		start := addMonthsClamped(covered, i-1).AddDate(0, 0, 1)
		end := addMonthsClamped(covered, i)
		if settings.ClosedAt != nil && start.After(*settings.ClosedAt) {
			break
		}
		if releaseDate(end, settings.ReleaseDay).After(today) {
			break
		}
		out = append(out, coveragePeriod{Start: start, End: end, Status: pb.StatementCoverageStatus_STATEMENT_COVERAGE_STATUS_DUE})
	}

	return out
}

// monthlyPeriods splits from..to into month-long periods starting at from, the last cut short at to.
func monthlyPeriods(from, to time.Time, status pb.StatementCoverageStatus) []coveragePeriod {
	var out []coveragePeriod
	for i := 0; i < maxCoveragePeriods; i++ {
		start := addMonthsClamped(from, i)
		if start.After(to) {
			break
		}
		end := addMonthsClamped(from, i+1).AddDate(0, 0, -1)
		if end.After(to) {
			end = to
		}
		out = append(out, coveragePeriod{Start: start, End: end, Status: status})
	}
	return out
}

// releaseDate is when the statement for a period ending on end comes out: the first
// release day after it, or a few days after it when the account has none.
func releaseDate(end time.Time, releaseDay *int16) time.Time {
	if releaseDay == nil {
		return end.AddDate(0, 0, statementReleaseLagDays)
	}
	release := dayInMonth(end.Year(), end.Month(), int(*releaseDay))
	if !release.After(end) {
		release = dayInMonth(end.Year(), end.Month()+1, int(*releaseDay))
	}
	return release
}

// addMonthsClamped moves t by n months, keeping its day where the month is long
// enough (jan 31 + 1 month is feb 28, not mar 3).
func addMonthsClamped(t time.Time, n int) time.Time {
	return dayInMonth(t.Year(), t.Month()+time.Month(n), t.Day())
}

// dayInMonth is the day of the month, or its last day when the month is shorter.
// month may run past december or before january.
func dayInMonth(year int, month time.Month, day int) time.Time {
	firstOfMonth := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	lastDay := firstOfMonth.AddDate(0, 1, -1).Day()
	return firstOfMonth.AddDate(0, 0, min(day, lastDay)-1)
}
