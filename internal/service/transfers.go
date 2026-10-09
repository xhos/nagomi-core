package service

import (
	"context"
	"fmt"
	"time"

	"nagomi-core/internal/db/sqlc"
	pb "nagomi-core/internal/gen/nagomi/v1"

	"github.com/charmbracelet/log"
	"github.com/google/uuid"
)

type TransferService interface {
	// Match pairs transfers around the given transactions.
	Match(ctx context.Context, userID uuid.UUID, transactionIDs []int64)
	// MatchAll runs Match over every user's history.
	MatchAll(ctx context.Context)
	Link(ctx context.Context, userID uuid.UUID, outgoingID, incomingID int64) error
	Unlink(ctx context.Context, userID uuid.UUID, transactionID int64) error
	ListSuggestions(ctx context.Context, userID uuid.UUID) ([]*pb.TransferSuggestion, error)
}

type transferSvc struct {
	queries *sqlc.Queries
	log     *log.Logger
}

func newTransferSvc(queries *sqlc.Queries, logger *log.Logger) TransferService {
	return &transferSvc{queries: queries, log: logger}
}

func (s *transferSvc) MatchAll(ctx context.Context) {
	users, err := s.queries.ListUsers(ctx)
	if err != nil {
		s.log.Error("failed to list users for transfer matching", "error", err)
		return
	}
	for _, u := range users {
		// nil ids span all of the user's history
		if err := s.match(ctx, u.ID, nil); err != nil {
			s.log.Warn("transfer matching failed", "user_id", u.ID, "error", err)
		}
	}
}

func (s *transferSvc) Match(ctx context.Context, userID uuid.UUID, transactionIDs []int64) {
	if len(transactionIDs) == 0 {
		return
	}
	if err := s.match(ctx, userID, transactionIDs); err != nil {
		s.log.Warn("transfer matching failed", "user_id", userID, "error", err)
	}
}

func (s *transferSvc) match(ctx context.Context, userID uuid.UUID, transactionIDs []int64) error {
	span, err := s.queries.GetTransactionDateRange(ctx, sqlc.GetTransactionDateRangeParams{UserID: userID, Ids: transactionIDs})
	if err != nil {
		// no transactions in range
		return nil
	}
	// counterparts of transactions at the edge of the range sit up to
	// transferMaxDays further out, and theirs further still
	margin := 2 * transferMaxDays * 24 * time.Hour
	rows, err := s.queries.ListTransferCandidates(ctx, sqlc.ListTransferCandidatesParams{
		UserID:    userID,
		StartDate: span.StartDate.Add(-margin),
		EndDate:   span.EndDate.Add(margin),
	})
	if err != nil {
		return fmt.Errorf("list candidates: %w", err)
	}
	loc := time.UTC
	if user, err := s.queries.GetUser(ctx, userID); err == nil {
		if l, err := time.LoadLocation(user.Timezone); err == nil {
			loc = l
		}
	}
	accountRows, err := s.queries.ListTransferAccounts(ctx, userID)
	if err != nil {
		return fmt.Errorf("list accounts: %w", err)
	}

	ids := make([]int64, len(rows))
	txs := make([]transferTx, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
		text := ""
		if r.TxDesc != nil {
			text = *r.TxDesc
		}
		if r.Merchant != nil {
			text += " " + *r.Merchant
		}
		txs[i] = transferTx{
			ID:        r.ID,
			AccountID: r.AccountID,
			// compared as calendar days where the user lives
			Date:      calendarDay(r.TxDate, loc),
			Cents:     r.TxAmountCents,
			Currency:  r.TxCurrency,
			Direction: r.TxDirection,
			Text:      text,
			Ref:       r.TransferRef,
		}
	}

	accounts := make(map[int64]transferAccount, len(accountRows))
	for _, a := range accountRows {
		names := append([]string{a.Name}, a.Aliases...)
		if a.FriendlyName != nil {
			names = append(names, *a.FriendlyName)
		}
		accounts[a.ID] = transferAccount{
			ID:         a.ID,
			Bank:       a.Bank,
			Names:      names,
			CreditCard: a.AccountType == pb.AccountType_ACCOUNT_CREDIT_CARD,
		}
	}

	existing, err := s.queries.ListTransfersTouching(ctx, sqlc.ListTransfersTouchingParams{UserID: userID, Ids: ids})
	if err != nil {
		return fmt.Errorf("list existing: %w", err)
	}
	linked := map[int64]bool{}
	rejected := map[[2]int64]bool{}
	for _, t := range existing {
		switch pb.TransferStatus(t.Status) {
		case pb.TransferStatus_TRANSFER_STATUS_LINKED:
			linked[t.OutTxID], linked[t.InTxID] = true, true
		case pb.TransferStatus_TRANSFER_STATUS_REJECTED:
			rejected[[2]int64{t.OutTxID, t.InTxID}] = true
		}
	}

	plan := planTransfers(txs, accounts, linked, rejected)

	// suggestions are recomputed every time
	if err := s.queries.DeleteSuggestedTransfers(ctx, sqlc.DeleteSuggestedTransfersParams{UserID: userID, Ids: ids}); err != nil {
		return fmt.Errorf("clear suggestions: %w", err)
	}
	save := func(pairs []transferPair, status pb.TransferStatus) {
		for _, p := range pairs {
			if err := s.queries.CreateTransfer(ctx, sqlc.CreateTransferParams{
				UserID:  userID,
				OutTxID: p.Out,
				InTxID:  p.In,
				Status:  int16(status),
				Method:  int16(p.Method),
			}); err != nil {
				s.log.Warn("failed to save transfer", "out", p.Out, "in", p.In, "error", err)
			}
		}
	}
	save(plan.Links, pb.TransferStatus_TRANSFER_STATUS_LINKED)
	save(plan.Suggestions, pb.TransferStatus_TRANSFER_STATUS_SUGGESTED)

	if len(plan.Links) > 0 || len(plan.Suggestions) > 0 {
		s.log.Debug("matched transfers", "user_id", userID, "linked", len(plan.Links), "suggested", len(plan.Suggestions))
	}
	return nil
}

func (s *transferSvc) Link(ctx context.Context, userID uuid.UUID, outgoingID, incomingID int64) error {
	out, err := s.queries.GetTransaction(ctx, sqlc.GetTransactionParams{UserID: userID, ID: outgoingID})
	if err != nil {
		return wrapErr("TransferService.Link.GetOutgoing", err)
	}
	in, err := s.queries.GetTransaction(ctx, sqlc.GetTransactionParams{UserID: userID, ID: incomingID})
	if err != nil {
		return wrapErr("TransferService.Link.GetIncoming", err)
	}
	if out.TxDirection != pb.TransactionDirection_DIRECTION_OUTGOING || in.TxDirection != pb.TransactionDirection_DIRECTION_INCOMING {
		return fmt.Errorf("TransferService.Link: a transfer goes from an outgoing to an incoming transaction: %w", ErrValidation)
	}
	if out.AccountID == in.AccountID {
		return fmt.Errorf("TransferService.Link: both sides are on the same account: %w", ErrValidation)
	}

	ids := []int64{outgoingID, incomingID}
	n, err := s.queries.CountLinkedTransfers(ctx, ids)
	if err != nil {
		return wrapErr("TransferService.Link.Count", err)
	}
	if n > 0 {
		return fmt.Errorf("TransferService.Link: a side is already part of a transfer; unlink it first: %w", ErrValidation)
	}

	if err := s.queries.DeleteSuggestedTransfers(ctx, sqlc.DeleteSuggestedTransfersParams{UserID: userID, Ids: ids}); err != nil {
		return wrapErr("TransferService.Link.ClearSuggestions", err)
	}
	if err := s.queries.LinkTransferManually(ctx, sqlc.LinkTransferManuallyParams{UserID: userID, OutTxID: outgoingID, InTxID: incomingID}); err != nil {
		return wrapErr("TransferService.Link", err)
	}
	return nil
}

func (s *transferSvc) Unlink(ctx context.Context, userID uuid.UUID, transactionID int64) error {
	n, err := s.queries.RejectTransfer(ctx, sqlc.RejectTransferParams{UserID: userID, TxID: transactionID})
	if err != nil {
		return wrapErr("TransferService.Unlink", err)
	}
	if n == 0 {
		return fmt.Errorf("TransferService.Unlink: transaction %d isn't part of a transfer: %w", transactionID, ErrValidation)
	}
	// each side may pair with something else now
	s.Match(ctx, userID, []int64{transactionID})
	return nil
}

func (s *transferSvc) ListSuggestions(ctx context.Context, userID uuid.UUID) ([]*pb.TransferSuggestion, error) {
	rows, err := s.queries.ListTransferSuggestions(ctx, userID)
	if err != nil {
		return nil, wrapErr("TransferService.ListSuggestions", err)
	}
	out := make([]*pb.TransferSuggestion, len(rows))
	for i := range rows {
		out[i] = &pb.TransferSuggestion{
			Outgoing: transactionToPb(&rows[i].Transaction),
			Incoming: transactionToPb(&rows[i].Transaction_2),
		}
	}
	return out, nil
}

// attachTransfers fills in the transfer each transaction belongs to.
func attachTransfers(ctx context.Context, q *sqlc.Queries, txs []*pb.Transaction) error {
	if len(txs) == 0 {
		return nil
	}
	ids := make([]int64, len(txs))
	for i, t := range txs {
		ids[i] = t.Id
	}
	rows, err := q.ListLinkedTransfers(ctx, ids)
	if err != nil {
		return err
	}
	byTx := make(map[int64]*pb.Transfer, 2*len(rows))
	for _, r := range rows {
		base := func(counterpart, account int64) *pb.Transfer {
			return &pb.Transfer{
				Id:                   r.ID,
				CounterpartId:        counterpart,
				CounterpartAccountId: account,
				Method:               pb.TransferMethod(r.Method),
			}
		}
		out, in := base(r.InID, r.InAccountID), base(r.OutID, r.OutAccountID)
		if r.OutCurrency == r.InCurrency && r.OutAmountCents > r.InAmountCents {
			lost := centsToMoney(r.OutAmountCents-r.InAmountCents, r.OutCurrency)
			out.Fee, in.Fee = lost, lost
		}
		byTx[r.OutID], byTx[r.InID] = out, in
	}
	for _, t := range txs {
		t.Transfer = byTx[t.Id]
	}
	return nil
}
