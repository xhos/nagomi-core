package service

import (
	"context"
	"errors"
	"fmt"
	"math"

	"nagomi-core/internal/db/sqlc"
	"nagomi-core/internal/exchange"
	pb "nagomi-core/internal/gen/nagomi/v1"

	"github.com/charmbracelet/log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ----- interface ---------------------------------------------------------------------------

type TransactionService interface {
	Create(ctx context.Context, userID uuid.UUID, req *pb.CreateTransactionRequest) ([]*pb.Transaction, error)
	Get(ctx context.Context, userID uuid.UUID, id int64) (*pb.Transaction, error)
	Update(ctx context.Context, userID uuid.UUID, req *pb.UpdateTransactionRequest) error
	Delete(ctx context.Context, userID uuid.UUID, ids []int64) error
	List(ctx context.Context, userID uuid.UUID, req *pb.ListTransactionsRequest) ([]*pb.Transaction, *pb.Cursor, error)
	Count(ctx context.Context, userID uuid.UUID, req *pb.ListTransactionsRequest) (int64, error)
	Categorize(ctx context.Context, userID uuid.UUID, transactionIDs []int64, categoryID int64) error
	SplitTransaction(ctx context.Context, userID uuid.UUID, req *pb.SplitTransactionRequest) ([]*pb.Transaction, error)
	ForgiveTransaction(ctx context.Context, userID uuid.UUID, transactionID int64, forgiven bool) error
	GetFriendBalances(ctx context.Context, userID uuid.UUID) ([]*pb.FriendBalance, error)
	// ApplyRules runs the user's rules over transactions created outside Create.
	ApplyRules(ctx context.Context, userID uuid.UUID, transactionIDs []int64)
}

type txnSvc struct {
	queries        *sqlc.Queries
	log            *log.Logger
	catSvc         CategoryService
	ruleSvc        RuleService
	exchangeClient *exchange.Client
}

func newTxnSvc(
	queries *sqlc.Queries,
	logger *log.Logger,
	catSvc CategoryService,
	ruleSvc RuleService,
	exchangeClient *exchange.Client,
) TransactionService {
	return &txnSvc{
		queries:        queries,
		log:            logger,
		catSvc:         catSvc,
		ruleSvc:        ruleSvc,
		exchangeClient: exchangeClient,
	}
}

// ----- methods -----------------------------------------------------------------------------

func (s *txnSvc) Create(ctx context.Context, userID uuid.UUID, req *pb.CreateTransactionRequest) ([]*pb.Transaction, error) {
	paramsList := buildCreateTxParamsList(userID, req)
	hasNoTransactions := len(paramsList) == 0
	if hasNoTransactions {
		return nil, fmt.Errorf("TransactionService.Create: no transactions provided")
	}

	for i, params := range paramsList {
		if err := validateCreateParams(params); err != nil {
			return nil, fmt.Errorf("TransactionService.Create: transaction %d invalid: %w", i, err)
		}
	}

	for i := range paramsList {
		converted, err := s.processForeignCurrency(ctx, userID, &paramsList[i])
		if err != nil {
			return nil, fmt.Errorf("TransactionService.Create: transaction %d currency conversion failed: %w", i, err)
		}
		paramsList[i] = *converted
	}

	created := make([]sqlc.Transaction, 0, len(paramsList))
	for _, params := range paramsList {
		tx, err := s.queries.CreateTransaction(ctx, params)
		if err != nil {
			// duplicates are ignored
			if errors.Is(err, pgx.ErrNoRows) && params.ExternalID != nil {
				s.log.Debug("skipping duplicate transaction", "external_id", *params.ExternalID, "account_id", params.AccountID)
				continue
			}
			return nil, wrapErr("TransactionService.Create.Insert", err)
		}
		created = append(created, tx)
	}

	affectedAccounts := make(map[int64]bool)
	for _, tx := range created {
		affectedAccounts[tx.AccountID] = true
	}
	for accountID := range affectedAccounts {
		if err := s.queries.SyncAccountBalances(ctx, accountID); err != nil {
			s.log.Warn("failed to sync account balances", "account_id", accountID, "error", err)
		}
	}

	for _, tx := range created {
		needsRules := !tx.CategoryManuallySet || !tx.MerchantManuallySet
		if needsRules {
			s.applyRulesToTransaction(ctx, userID, tx.ID)
		}
	}

	result := make([]*pb.Transaction, len(created))
	for i := range created {
		result[i] = transactionToPb(&created[i])
	}

	return result, nil
}

func (s *txnSvc) ApplyRules(ctx context.Context, userID uuid.UUID, transactionIDs []int64) {
	for _, id := range transactionIDs {
		s.applyRulesToTransaction(ctx, userID, id)
	}
}

func (s *txnSvc) Get(ctx context.Context, userID uuid.UUID, id int64) (*pb.Transaction, error) {
	row, err := s.queries.GetTransaction(ctx, sqlc.GetTransactionParams{UserID: userID, ID: id})
	if err != nil {
		return nil, wrapErr("TransactionService.Get", err)
	}

	proto := transactionToPb(&row)

	if row.SplitFromID == nil {
		splits, err := s.queries.GetSplitsBySourceID(ctx, row.ID)
		if err == nil && len(splits) > 0 {
			proto.Splits = make([]*pb.Transaction, len(splits))
			for i := range splits {
				proto.Splits[i] = transactionToPb(&splits[i])
			}
		}
	}

	return proto, nil
}

func (s *txnSvc) Update(ctx context.Context, userID uuid.UUID, req *pb.UpdateTransactionRequest) error {
	if err := validateUpdateRequest(req); err != nil {
		return err
	}

	params := buildUpdateTxParams(userID, req)

	tx, err := s.queries.GetTransaction(ctx, sqlc.GetTransactionParams{UserID: params.UserID, ID: params.ID})
	if err != nil {
		return wrapErr("TransactionService.Update.GetOriginal", err)
	}

	if err := s.queries.UpdateTransaction(ctx, params); err != nil {
		return wrapErr("TransactionService.Update", err)
	}

	balanceFieldsChanged := params.TxAmountCents != nil || params.TxDate != nil || params.TxDirection != nil
	accountChanged := params.AccountID != nil && *params.AccountID != tx.AccountID
	shouldSyncBalances := balanceFieldsChanged || accountChanged
	if shouldSyncBalances {
		if err := s.queries.SyncAccountBalances(ctx, tx.AccountID); err != nil {
			s.log.Warn("failed to sync original account balances after updating transaction", "tx_id", params.ID, "account_id", tx.AccountID, "error", err)
		}
		if accountChanged {
			if err := s.queries.SyncAccountBalances(ctx, *params.AccountID); err != nil {
				s.log.Warn("failed to sync new account balances after updating transaction", "tx_id", params.ID, "account_id", *params.AccountID, "error", err)
			}
		}
	}

	fieldsChangedForRules := params.TxDesc != nil || params.Merchant != nil || params.TxAmountCents != nil
	rulesCanMutate := !tx.CategoryManuallySet || !tx.MerchantManuallySet
	shouldApplyRules := fieldsChangedForRules && rulesCanMutate
	if shouldApplyRules {
		s.applyRulesToTransaction(ctx, params.UserID, params.ID)
	}

	if params.TxAmountCents != nil {
		s.adjustSplitsProportionally(ctx, userID, tx.ID, tx.TxAmountCents, *params.TxAmountCents)
	}

	return nil
}

func (s *txnSvc) Delete(ctx context.Context, userID uuid.UUID, ids []int64) error {
	affectedAccounts, err := s.queries.GetAccountIDsFromTransactionIDs(ctx, ids)
	if err != nil {
		return wrapErr("TransactionService.BulkDelete.GetAccounts", err)
	}

	friendAccounts, err := s.queries.GetFriendAccountIDsFromSplits(ctx, ids)
	if err != nil {
		s.log.Warn("failed to get friend accounts from splits before delete", "error", err)
	}

	_, err = s.queries.BulkDeleteTransactions(ctx, sqlc.BulkDeleteTransactionsParams{UserID: userID, TransactionIds: ids})
	if err != nil {
		return wrapErr("TransactionService.BulkDelete", err)
	}

	for _, accountID := range affectedAccounts {
		if err := s.queries.SyncAccountBalances(ctx, accountID); err != nil {
			s.log.Warn("failed to sync account balances after bulk delete", "account_id", accountID, "error", err)
		}
	}
	for _, accountID := range friendAccounts {
		if err := s.queries.SyncAccountBalances(ctx, accountID); err != nil {
			s.log.Warn("failed to sync friend account balances after bulk delete", "account_id", accountID, "error", err)
		}
	}

	s.log.Debug("bulk deleted transactions and synced balances", "affected_accounts", len(affectedAccounts))

	return nil
}

func (s *txnSvc) List(ctx context.Context, userID uuid.UUID, req *pb.ListTransactionsRequest) ([]*pb.Transaction, *pb.Cursor, error) {
	params := buildListTxParams(userID, req)

	rows, err := s.queries.ListTransactions(ctx, params)
	if err != nil {
		return nil, nil, wrapErr("TransactionService.List", err)
	}

	result := make([]*pb.Transaction, len(rows))
	for i := range rows {
		result[i] = transactionToPb(&rows[i].Transaction)
		if rows[i].ReceiptID != 0 {
			result[i].ReceiptId = &rows[i].ReceiptID
		}
	}

	var nextCursor *pb.Cursor
	hasResults := len(result) > 0
	hasLimit := req.Limit != nil
	pageIsFull := hasLimit && len(result) == int(*req.Limit)
	hasNextPage := hasResults && pageIsFull
	if hasNextPage {
		lastTx := result[len(result)-1]
		id := lastTx.Id
		nextCursor = &pb.Cursor{Date: lastTx.TxDate, Id: &id}
	}

	return result, nextCursor, nil
}

func (s *txnSvc) Count(ctx context.Context, userID uuid.UUID, req *pb.ListTransactionsRequest) (int64, error) {
	p := buildListTxParams(userID, req)
	return s.queries.CountTransactions(ctx, sqlc.CountTransactionsParams{
		UserID: p.UserID, Start: p.Start, End: p.End,
		AmountMinCents: p.AmountMinCents, AmountMaxCents: p.AmountMaxCents,
		Direction: p.Direction, AccountIds: p.AccountIds, Categories: p.Categories,
		MerchantQ: p.MerchantQ, DescQ: p.DescQ, Currency: p.Currency,
		TodStart: p.TodStart, TodEnd: p.TodEnd, Uncategorized: p.Uncategorized,
	})
}

func (s *txnSvc) Categorize(ctx context.Context, userID uuid.UUID, transactionIDs []int64, categoryID int64) error {
	_, err := s.queries.BulkCategorizeTransactions(ctx, sqlc.BulkCategorizeTransactionsParams{
		UserID:         userID,
		TransactionIds: transactionIDs,
		CategoryID:     categoryID,
	})
	if err != nil {
		return wrapErr("TransactionService.Categorize", err)
	}

	return nil
}

func (s *txnSvc) SplitTransaction(ctx context.Context, userID uuid.UUID, req *pb.SplitTransactionRequest) ([]*pb.Transaction, error) {
	sourceTx, err := s.queries.GetTransaction(ctx, sqlc.GetTransactionParams{UserID: userID, ID: req.SourceTransactionId})
	if err != nil {
		return nil, wrapErr("TransactionService.Split.GetSource", err)
	}
	if sourceTx.SplitFromID != nil {
		return nil, fmt.Errorf("TransactionService.Split: cannot split a transaction that is itself a split: %w", ErrValidation)
	}

	for _, entry := range req.Splits {
		account, err := s.queries.GetAccount(ctx, sqlc.GetAccountParams{UserID: userID, ID: entry.FriendAccountId})
		if err != nil {
			return nil, wrapErr("TransactionService.Split.GetFriendAccount", err)
		}
		if account.Account.AccountType != 6 {
			return nil, fmt.Errorf("TransactionService.Split: account %d is not a friend account: %w", entry.FriendAccountId, ErrValidation)
		}
	}

	// Replace existing splits: collect affected accounts, then delete
	existingSplits, err := s.queries.GetSplitsBySourceID(ctx, sourceTx.ID)
	if err != nil {
		return nil, wrapErr("TransactionService.Split.GetExisting", err)
	}

	affectedAccounts := make(map[int64]bool)
	if len(existingSplits) > 0 {
		existingIDs := make([]int64, len(existingSplits))
		for i, split := range existingSplits {
			existingIDs[i] = split.ID
			affectedAccounts[split.AccountID] = true
		}
		if _, err := s.queries.BulkDeleteTransactions(ctx, sqlc.BulkDeleteTransactionsParams{
			UserID:         userID,
			TransactionIds: existingIDs,
		}); err != nil {
			return nil, wrapErr("TransactionService.Split.DeleteExisting", err)
		}
	}

	sourceID := sourceTx.ID
	created := make([]*pb.Transaction, 0, len(req.Splits))

	categoryManuallySet := false
	merchantManuallySet := false

	for _, entry := range req.Splits {
		params := sqlc.CreateTransactionParams{
			UserID:              userID,
			AccountID:           entry.FriendAccountId,
			TxDate:              sourceTx.TxDate,
			TxAmountCents:       moneyToCents(entry.Amount),
			TxCurrency:          sourceTx.TxCurrency,
			TxDirection:         1, // incoming = adds to what they owe you
			TxDesc:              sourceTx.TxDesc,
			Merchant:            sourceTx.Merchant,
			CategoryID:          sourceTx.CategoryID,
			CategoryManuallySet: &categoryManuallySet,
			MerchantManuallySet: &merchantManuallySet,
			SplitFromID:         &sourceID,
			Source:              int16(pb.TransactionSource_TRANSACTION_SOURCE_SPLIT),
		}

		processed, err := s.processForeignCurrency(ctx, userID, &params)
		if err != nil {
			return nil, wrapErr("TransactionService.Split.Currency", err)
		}

		tx, err := s.queries.CreateTransaction(ctx, *processed)
		if err != nil {
			return nil, wrapErr("TransactionService.Split.Create", err)
		}
		created = append(created, transactionToPb(&tx))
		affectedAccounts[entry.FriendAccountId] = true
	}

	for accountID := range affectedAccounts {
		if err := s.queries.SyncAccountBalances(ctx, accountID); err != nil {
			s.log.Warn("failed to sync friend account balances after split", "account_id", accountID, "error", err)
		}
	}

	return created, nil
}

func (s *txnSvc) ForgiveTransaction(ctx context.Context, userID uuid.UUID, transactionID int64, forgiven bool) error {
	tx, err := s.queries.GetTransaction(ctx, sqlc.GetTransactionParams{UserID: userID, ID: transactionID})
	if err != nil {
		return wrapErr("TransactionService.Forgive.Get", err)
	}

	err = s.queries.UpdateTransactionForgiven(ctx, sqlc.UpdateTransactionForgivenParams{
		Forgiven: forgiven,
		ID:       transactionID,
		UserID:   userID,
	})
	if err != nil {
		return wrapErr("TransactionService.Forgive", err)
	}

	if err := s.queries.SyncAccountBalances(ctx, tx.AccountID); err != nil {
		s.log.Warn("failed to sync account balances after forgive", "account_id", tx.AccountID, "error", err)
	}

	return nil
}

func (s *txnSvc) GetFriendBalances(ctx context.Context, userID uuid.UUID) ([]*pb.FriendBalance, error) {
	rows, err := s.queries.GetFriendAccountBalances(ctx, userID)
	if err != nil {
		return nil, wrapErr("TransactionService.GetFriendBalances", err)
	}

	result := make([]*pb.FriendBalance, len(rows))
	for i, row := range rows {
		result[i] = &pb.FriendBalance{
			AccountId:  row.ID,
			FriendName: row.Name,
			Balance:    centsToMoney(row.BalanceCents, row.Currency),
		}
	}

	return result, nil
}

func (s *txnSvc) adjustSplitsProportionally(ctx context.Context, userID uuid.UUID, sourceID int64, oldAmountCents, newAmountCents int64) {
	if oldAmountCents == 0 {
		return
	}

	splits, err := s.queries.GetSplitsBySourceID(ctx, sourceID)
	if err != nil || len(splits) == 0 {
		return
	}

	ratio := float64(newAmountCents) / float64(oldAmountCents)

	for _, split := range splits {
		adjustedCents := int64(math.Round(float64(split.TxAmountCents) * ratio))
		err := s.queries.UpdateTransaction(ctx, sqlc.UpdateTransactionParams{
			ID:            split.ID,
			UserID:        userID,
			TxAmountCents: &adjustedCents,
		})
		if err != nil {
			s.log.Warn("failed to adjust split proportionally", "split_id", split.ID, "error", err)
			continue
		}
		if err := s.queries.SyncAccountBalances(ctx, split.AccountID); err != nil {
			s.log.Warn("failed to sync friend account after split adjustment", "account_id", split.AccountID, "error", err)
		}
	}
}
