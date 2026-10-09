package service

import (
	"context"
	"errors"
	"math"
	"slices"
	"sort"
	"time"

	"nagomi-core/internal/db/sqlc"
	pb "nagomi-core/internal/gen/nagomi/v1"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const (
	// how far a statement line's date may sit from the transaction it confirms
	reconcileDateWindow = 3
	// second pass only: tips and fx make the statement amount differ from the provisional one
	reconcileAmountTolerance = 0.25
	// provisional transactions this close to the period end may still post on the next statement
	reconcileGraceDays = 5
)

// reconcileLine is a statement line, dated as a calendar day.
type reconcileLine struct {
	Day         time.Time
	AmountCents int64
	Direction   pb.TransactionDirection
}

// reconcileCandidate is an account transaction from around the statement period that
// no statement has confirmed yet, dated as a calendar day.
type reconcileCandidate struct {
	ID          int64
	Day         time.Time
	AmountCents int64
	Direction   pb.TransactionDirection
	Source      pb.TransactionSource
	// notes, a receipt or splits: never deleted automatically
	HasUserData bool
	// already a line of the statement being re-parsed, so deleted when the new
	// lines leave it out, whatever its source or date
	FromStatement bool
}

type reconcileMatch struct {
	Line          int
	TransactionID int64
	AmountChanged bool
}

type reconcileKept struct {
	TransactionID int64
	Reason        pb.ReconciliationKeepReason
}

type reconcilePlan struct {
	Matches []reconcileMatch
	// indexes of lines with no transaction to confirm
	Create []int
	Delete []int64
	Keep   []reconcileKept
}

// planReconcile pairs statement lines with provisional transactions: exact amounts
// first, closest date wins; then, for what's left, amounts within the tolerance when
// the pairing is unambiguous. unmatched provisional transactions inside the period
// are deleted unless they're manual, carry user data, or fall in the grace window.
// candidates outside the period that didn't match are left alone.
// when re-parsing, the statement's own unmatched transactions are deleted unless
// they carry user data.
func planReconcile(lines []reconcileLine, candidates []reconcileCandidate, periodStart, periodEnd time.Time) reconcilePlan {
	var plan reconcilePlan
	lineUsed := make([]bool, len(lines))
	candUsed := make([]bool, len(candidates))

	// pass 1: exact amount, globally closest dates first
	type pair struct{ line, cand, dist int }
	var exact []pair
	for i, line := range lines {
		for j, cand := range candidates {
			dist := dayDistance(line.Day, cand.Day)
			if cand.Direction == line.Direction && cand.AmountCents == line.AmountCents && dist <= reconcileDateWindow {
				exact = append(exact, pair{i, j, dist})
			}
		}
	}
	sort.SliceStable(exact, func(a, b int) bool { return exact[a].dist < exact[b].dist })
	for _, p := range exact {
		if lineUsed[p.line] || candUsed[p.cand] {
			continue
		}
		lineUsed[p.line], candUsed[p.cand] = true, true
		plan.Matches = append(plan.Matches, reconcileMatch{Line: p.line, TransactionID: candidates[p.cand].ID})
	}

	// pass 2: amount within tolerance, only when line and transaction have no other option
	near := func(line reconcileLine, cand reconcileCandidate) bool {
		diff := line.AmountCents - cand.AmountCents
		if diff < 0 {
			diff = -diff
		}
		return cand.Direction == line.Direction &&
			dayDistance(line.Day, cand.Day) <= reconcileDateWindow &&
			float64(diff) <= reconcileAmountTolerance*float64(line.AmountCents)
	}
	lineOptions := make([][]int, len(lines))
	candOptions := make([]int, len(candidates))
	for i, line := range lines {
		if lineUsed[i] {
			continue
		}
		for j, cand := range candidates {
			if !candUsed[j] && near(line, cand) {
				lineOptions[i] = append(lineOptions[i], j)
				candOptions[j]++
			}
		}
	}
	for i, options := range lineOptions {
		if len(options) != 1 || candOptions[options[0]] != 1 {
			continue
		}
		j := options[0]
		lineUsed[i], candUsed[j] = true, true
		plan.Matches = append(plan.Matches, reconcileMatch{Line: i, TransactionID: candidates[j].ID, AmountChanged: true})
	}
	sort.Slice(plan.Matches, func(a, b int) bool { return plan.Matches[a].Line < plan.Matches[b].Line })

	for i := range lines {
		if !lineUsed[i] {
			plan.Create = append(plan.Create, i)
		}
	}

	graceFrom := periodEnd.AddDate(0, 0, -reconcileGraceDays)
	for j, cand := range candidates {
		if candUsed[j] {
			continue
		}
		if cand.FromStatement {
			if cand.HasUserData {
				plan.Keep = append(plan.Keep, reconcileKept{cand.ID, pb.ReconciliationKeepReason_RECONCILIATION_KEEP_REASON_USER_DATA})
			} else {
				plan.Delete = append(plan.Delete, cand.ID)
			}
			continue
		}
		if cand.Day.Before(periodStart) || cand.Day.After(periodEnd) {
			continue
		}
		// TODO: connector accounts are never statement-driven, so CONNECTOR can't show up
		// here; drop it once core enforces that (see buildUpdateAccountParams)
		provisional := cand.Source == pb.TransactionSource_TRANSACTION_SOURCE_EMAIL ||
			cand.Source == pb.TransactionSource_TRANSACTION_SOURCE_CONNECTOR
		switch {
		case !provisional:
			plan.Keep = append(plan.Keep, reconcileKept{cand.ID, pb.ReconciliationKeepReason_RECONCILIATION_KEEP_REASON_MANUAL})
		case cand.HasUserData:
			plan.Keep = append(plan.Keep, reconcileKept{cand.ID, pb.ReconciliationKeepReason_RECONCILIATION_KEEP_REASON_USER_DATA})
		case cand.Day.After(graceFrom):
			plan.Keep = append(plan.Keep, reconcileKept{cand.ID, pb.ReconciliationKeepReason_RECONCILIATION_KEEP_REASON_GRACE})
		default:
			plan.Delete = append(plan.Delete, cand.ID)
		}
	}

	return plan
}

// calendarDay is midnight UTC of t's date in loc, so days compare without zone drift.
func calendarDay(t time.Time, loc *time.Location) time.Time {
	y, m, d := t.In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func dayDistance(a, b time.Time) int {
	days := int(a.Sub(b).Hours() / 24)
	if days < 0 {
		return -days
	}
	return days
}

// ----- against the database ----------------------------------------------------------------

// reconciliation is a plan together with what it was built from, so commit can apply it.
type reconciliation struct {
	accountID   int64
	plan        reconcilePlan
	lines       []*pb.ParsedStatementLine
	externalIDs []string
	// line indexes this or an overlapping statement already imported
	alreadyImported map[int]bool
	candidates      map[int64]sqlc.ListReconcileCandidatesRow
}

// reconcileAccount plans an import of parsed into the account. reparsing is the
// statement being re-parsed, whose own transactions become candidates too.
func reconcileAccount(
	ctx context.Context,
	q *sqlc.Queries,
	parsed *pb.ParsedStatement,
	accountID int64,
	loc *time.Location,
	reparsing *int64,
) (*reconciliation, error) {
	r := &reconciliation{
		accountID:       accountID,
		lines:           parsed.GetLines(),
		externalIDs:     lineExternalIDs(parsed.GetLines()),
		alreadyImported: map[int]bool{},
		candidates:      map[int64]sqlc.ListReconcileCandidatesRow{},
	}

	existing, err := q.ListExistingExternalIDs(ctx, sqlc.ListExistingExternalIDsParams{AccountID: accountID, ExternalIds: r.externalIDs})
	if err != nil {
		return nil, wrapErr("StatementService.Reconcile.ListExisting", err)
	}
	imported := make(map[string]bool, len(existing))
	for _, id := range existing {
		imported[id] = true
	}

	// planReconcile sees only the lines still to import; index maps them back
	var lines []reconcileLine
	var index []int
	for i, line := range r.lines {
		if imported[r.externalIDs[i]] {
			r.alreadyImported[i] = true
			continue
		}
		lines = append(lines, reconcileLine{
			Day:         dateToUTC(line.GetDate()),
			AmountCents: line.GetAmountCents(),
			Direction:   line.GetDirection(),
		})
		index = append(index, i)
	}

	rows, err := q.ListReconcileCandidates(ctx, sqlc.ListReconcileCandidatesParams{
		AccountID:   accountID,
		FromDate:    dateIn(parsed.GetPeriodStart(), loc).AddDate(0, 0, -reconcileDateWindow),
		ToDate:      dateIn(parsed.GetPeriodEnd(), loc).AddDate(0, 0, reconcileDateWindow+1),
		StatementID: reparsing,
	})
	if err != nil {
		return nil, wrapErr("StatementService.Reconcile.ListCandidates", err)
	}
	candidates := make([]reconcileCandidate, 0, len(rows))
	for _, row := range rows {
		// already this statement's line: unchanged by a re-parse, or not linked to it yet
		if row.Transaction.ExternalID != nil && imported[*row.Transaction.ExternalID] {
			continue
		}
		r.candidates[row.Transaction.ID] = row
		candidates = append(candidates, reconcileCandidate{
			ID:            row.Transaction.ID,
			Day:           calendarDay(row.Transaction.TxDate, loc),
			AmountCents:   row.Transaction.TxAmountCents,
			Direction:     row.Transaction.TxDirection,
			Source:        pb.TransactionSource(row.Transaction.Source),
			HasUserData:   row.HasUserData,
			FromStatement: row.FromStatement,
		})
	}

	r.plan = planReconcile(lines, candidates, dateToUTC(parsed.GetPeriodStart()), dateToUTC(parsed.GetPeriodEnd()))
	for i := range r.plan.Matches {
		r.plan.Matches[i].Line = index[r.plan.Matches[i].Line]
	}
	for i := range r.plan.Create {
		r.plan.Create[i] = index[r.plan.Create[i]]
	}

	return r, nil
}

func (r *reconciliation) toPb() *pb.StatementReconciliation {
	matched := make(map[int]reconcileMatch, len(r.plan.Matches))
	for _, m := range r.plan.Matches {
		matched[m.Line] = m
	}

	out := &pb.StatementReconciliation{AccountId: r.accountID}
	for i := range r.lines {
		lineIndex := int32(i)
		item := &pb.ReconciliationItem{LineIndex: &lineIndex, Action: pb.ReconciliationAction_RECONCILIATION_ACTION_CREATE}
		if r.alreadyImported[i] {
			item.Action = pb.ReconciliationAction_RECONCILIATION_ACTION_ALREADY_IMPORTED
		} else if m, ok := matched[i]; ok {
			item.Action = pb.ReconciliationAction_RECONCILIATION_ACTION_CONFIRM
			if m.AmountChanged {
				item.Action = pb.ReconciliationAction_RECONCILIATION_ACTION_UPDATE_AMOUNT
			}
			item.Transaction = r.transactionPb(m.TransactionID)
		}
		out.Items = append(out.Items, item)
	}
	for _, id := range r.plan.Delete {
		out.Items = append(out.Items, &pb.ReconciliationItem{
			Action:      pb.ReconciliationAction_RECONCILIATION_ACTION_DELETE,
			Transaction: r.transactionPb(id),
		})
	}
	for _, kept := range r.plan.Keep {
		out.Items = append(out.Items, &pb.ReconciliationItem{
			Action:      pb.ReconciliationAction_RECONCILIATION_ACTION_KEEP,
			Transaction: r.transactionPb(kept.TransactionID),
			KeepReason:  kept.Reason,
		})
	}
	return out
}

func (r *reconciliation) transactionPb(id int64) *pb.Transaction {
	row := r.candidates[id]
	return transactionToPb(&row.Transaction)
}

type reconcileResult struct {
	createdIDs      []int64
	duplicates      int32
	updated         int32
	deleted         int32
	touchedAccounts map[int64]bool
}

// transactionIDs lists the transactions the statement created or confirmed.
func (r *reconciliation) transactionIDs(res reconcileResult) []int64 {
	ids := slices.Clone(res.createdIDs)
	for _, m := range r.plan.Matches {
		ids = append(ids, m.TransactionID)
	}
	return ids
}

// apply writes the plan inside the caller's db transaction.
func (r *reconciliation) apply(
	ctx context.Context,
	q *sqlc.Queries,
	userID uuid.UUID,
	stmt *sqlc.Statement,
	loc *time.Location,
) (reconcileResult, error) {
	res := reconcileResult{touchedAccounts: map[int64]bool{}}
	source := int16(pb.TransactionSource_TRANSACTION_SOURCE_STATEMENT)

	for _, m := range r.plan.Matches {
		line := r.lines[m.Line]
		tx := r.candidates[m.TransactionID].Transaction

		// keep the provisional time of day when the statement agrees on the day
		txDate := tx.TxDate
		if calendarDay(tx.TxDate, loc) != dateToUTC(line.GetDate()) {
			txDate = dateIn(line.GetDate(), loc)
		}
		// the statement shows what the bank actually charged for the foreign amount
		var rate *float64
		if m.AmountChanged && tx.ForeignAmountCents != nil && *tx.ForeignAmountCents != 0 {
			actual := float64(line.GetAmountCents()) / float64(*tx.ForeignAmountCents)
			rate = &actual
		}
		if line.ExchangeRate != nil {
			rate = line.ExchangeRate
		}

		if err := q.ConfirmStatementTransaction(ctx, sqlc.ConfirmStatementTransactionParams{
			ID:            tx.ID,
			AccountID:     r.accountID,
			TxDate:        txDate,
			TxAmountCents: line.GetAmountCents(),
			TxDesc:             line.GetDescription(),
			ForeignAmountCents: line.ForeignAmountCents,
			ForeignCurrency:    line.ForeignCurrency,
			ExchangeRate:       rate,
			ExternalID:         r.externalIDs[m.Line],
			StatementID:        stmt.ID,
			Source:             source,
		}); err != nil {
			return res, wrapErr("StatementService.Commit.Confirm", err)
		}
		if m.AmountChanged {
			res.updated++
			if r.candidates[m.TransactionID].HasSplits {
				if err := rescaleSplits(ctx, q, userID, tx.ID, tx.TxAmountCents, line.GetAmountCents(), res.touchedAccounts); err != nil {
					return res, err
				}
			}
		}
	}

	notManual := false
	for _, i := range r.plan.Create {
		line := r.lines[i]
		created, err := q.CreateTransaction(ctx, sqlc.CreateTransactionParams{
			UserID:              userID,
			AccountID:           r.accountID,
			ExternalID:          &r.externalIDs[i],
			TxDate:              dateIn(line.GetDate(), loc),
			TxAmountCents:       line.GetAmountCents(),
			TxCurrency:          stmt.Currency,
			TxDirection:         int16(line.GetDirection()),
			TxDesc:              &line.Description,
			ForeignAmountCents:  line.ForeignAmountCents,
			ForeignCurrency:     line.ForeignCurrency,
			ExchangeRate:        line.ExchangeRate,
			CategoryManuallySet: &notManual,
			MerchantManuallySet: &notManual,
			Source:              source,
			StatementID:         &stmt.ID,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			res.duplicates++
			continue
		}
		if err != nil {
			return res, wrapErr("StatementService.Commit.CreateTransaction", err)
		}
		res.createdIDs = append(res.createdIDs, created.ID)
	}
	res.duplicates += int32(len(r.alreadyImported))
	if len(r.alreadyImported) > 0 {
		ids := make([]string, 0, len(r.alreadyImported))
		for i := range r.alreadyImported {
			ids = append(ids, r.externalIDs[i])
		}
		if err := q.LinkStatementTransactions(ctx, sqlc.LinkStatementTransactionsParams{
			StatementID: stmt.ID,
			AccountID:   r.accountID,
			ExternalIds: ids,
		}); err != nil {
			return res, wrapErr("StatementService.Commit.Link", err)
		}
	}

	if len(r.plan.Delete) > 0 {
		deleted, err := q.BulkDeleteTransactions(ctx, sqlc.BulkDeleteTransactionsParams{UserID: userID, TransactionIds: r.plan.Delete})
		if err != nil {
			return res, wrapErr("StatementService.Commit.Delete", err)
		}
		res.deleted = int32(deleted)
	}

	return res, nil
}

// rescaleSplits keeps each friend's share proportional when the statement changes
// the amount, like editing the amount by hand does.
func rescaleSplits(ctx context.Context, q *sqlc.Queries, userID uuid.UUID, sourceID, oldCents, newCents int64, touched map[int64]bool) error {
	if oldCents == 0 {
		return nil
	}
	splits, err := q.GetSplitsBySourceID(ctx, sourceID)
	if err != nil {
		return wrapErr("StatementService.Commit.GetSplits", err)
	}
	ratio := float64(newCents) / float64(oldCents)
	for _, split := range splits {
		adjusted := int64(math.Round(float64(split.TxAmountCents) * ratio))
		if err := q.UpdateTransaction(ctx, sqlc.UpdateTransactionParams{ID: split.ID, UserID: userID, TxAmountCents: &adjusted}); err != nil {
			return wrapErr("StatementService.Commit.RescaleSplit", err)
		}
		touched[split.AccountID] = true
	}
	return nil
}
