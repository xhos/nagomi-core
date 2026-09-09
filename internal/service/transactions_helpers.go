package service

import (
	"context"
	"fmt"

	"nagomi-core/internal/db/sqlc"
	pb "nagomi-core/internal/gen/nagomi/v1"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func buildListTxParams(userID uuid.UUID, req *pb.ListTransactionsRequest) sqlc.ListTransactionsParams {
	params := sqlc.ListTransactionsParams{UserID: userID, Limit: req.Limit}

	if cursor := req.GetCursor(); cursor != nil {
		hasCursorDate := cursor.Date != nil
		hasCursorID := cursor.Id != nil
		cursorIsComplete := hasCursorDate && hasCursorID
		if cursorIsComplete {
			cursorTime := fromProtoTimestamp(cursor.Date)
			params.CursorDate = &cursorTime
			params.CursorID = cursor.Id
		}
	}

	if req.StartDate != nil {
		startTime := fromProtoTimestamp(req.StartDate)
		params.Start = &startTime
	}
	if req.EndDate != nil {
		endTime := fromProtoTimestamp(req.EndDate)
		params.End = &endTime
	}
	if req.AmountMin != nil {
		amountMinCents := moneyToCents(req.AmountMin)
		params.AmountMinCents = &amountMinCents
	}
	if req.AmountMax != nil {
		amountMaxCents := moneyToCents(req.AmountMax)
		params.AmountMaxCents = &amountMaxCents
	}
	hasDirection := req.Direction != nil
	directionIsSpecified := hasDirection && *req.Direction != pb.TransactionDirection_DIRECTION_UNSPECIFIED
	if directionIsSpecified {
		direction := int16(*req.Direction)
		params.Direction = &direction
	}
	if req.AccountId != nil {
		params.AccountIds = append(params.AccountIds, *req.AccountId)
	}
	if len(req.AccountIds) > 0 {
		params.AccountIds = append(params.AccountIds, req.AccountIds...)
	}
	if len(req.Categories) > 0 {
		params.Categories = req.Categories
	}
	if req.MerchantQuery != nil {
		params.MerchantQ = req.MerchantQuery
	}
	if req.DescriptionQuery != nil {
		params.DescQ = req.DescriptionQuery
	}
	if req.Currency != nil {
		params.Currency = req.Currency
	}
	if req.TimeOfDayStart != nil {
		params.TodStart = toPgTimeOfDay(req.TimeOfDayStart)
	}
	if req.TimeOfDayEnd != nil {
		params.TodEnd = toPgTimeOfDay(req.TimeOfDayEnd)
	}
	if req.Uncategorized != nil {
		params.Uncategorized = req.Uncategorized
	}

	return params
}

func buildCreateTxParamsList(userID uuid.UUID, req *pb.CreateTransactionRequest) []sqlc.CreateTransactionParams {
	paramsList := make([]sqlc.CreateTransactionParams, len(req.Transactions))

	for i, txInput := range req.Transactions {
		txAmount := txInput.TxAmount
		txCurrency := ""
		if txAmount != nil {
			txCurrency = txAmount.GetCurrencyCode()
		}

		categoryManuallySet := false
		merchantManuallySet := false

		params := sqlc.CreateTransactionParams{
			UserID:              userID,
			AccountID:           txInput.GetAccountId(),
			ExternalID:          txInput.ExternalId,
			TxDate:              fromProtoTimestamp(txInput.TxDate),
			TxAmountCents:       moneyToCents(txAmount),
			TxCurrency:          txCurrency,
			TxDirection:         int16(txInput.Direction),
			TxDesc:              txInput.Description,
			Merchant:            txInput.Merchant,
			UserNotes:           txInput.UserNotes,
			CategoryManuallySet: &categoryManuallySet,
			MerchantManuallySet: &merchantManuallySet,
		}

		if txInput.CategoryId != nil {
			params.CategoryID = txInput.CategoryId
			manuallySet := true
			params.CategoryManuallySet = &manuallySet
		}
		if txInput.Merchant != nil {
			manuallySet := true
			params.MerchantManuallySet = &manuallySet
		}

		if txInput.SplitFromId != nil {
			params.SplitFromID = txInput.SplitFromId
		}

		if txInput.ForeignAmount != nil {
			foreignCents := moneyToCents(txInput.ForeignAmount)
			foreignCurrency := txInput.ForeignAmount.CurrencyCode
			if foreignCurrency == "" {
				foreignCurrency = "USD"
			}
			params.ForeignAmountCents = &foreignCents
			params.ForeignCurrency = &foreignCurrency
			if txInput.ExchangeRate != nil {
				params.ExchangeRate = txInput.ExchangeRate
			}
		}

		paramsList[i] = params
	}

	return paramsList
}

func buildUpdateTxParams(userID uuid.UUID, req *pb.UpdateTransactionRequest) sqlc.UpdateTransactionParams {
	params := sqlc.UpdateTransactionParams{ID: req.GetId(), UserID: userID}

	if req.TxDate != nil {
		txTime := fromProtoTimestamp(req.TxDate)
		params.TxDate = &txTime
	}
	if req.TxAmount != nil {
		cents := moneyToCents(req.TxAmount)
		currency := req.TxAmount.CurrencyCode
		params.TxAmountCents = &cents
		params.TxCurrency = &currency
	}
	if req.Direction != nil && *req.Direction != pb.TransactionDirection_DIRECTION_UNSPECIFIED {
		direction := int16(*req.Direction)
		params.TxDirection = &direction
	}
	if req.Description != nil {
		params.TxDesc = req.Description
	}
	if req.Merchant != nil {
		params.Merchant = req.Merchant
		manuallySet := *req.Merchant != ""
		params.MerchantManuallySet = &manuallySet
	}
	if req.UserNotes != nil {
		params.UserNotes = req.UserNotes
	}
	if req.CategoryId != nil {
		params.CategoryID = req.CategoryId
		manuallySet := *req.CategoryId > 0
		params.CategoryManuallySet = &manuallySet
	}
	if req.ForeignAmount != nil {
		foreignCents := moneyToCents(req.ForeignAmount)
		foreignCurrency := req.ForeignAmount.CurrencyCode
		if foreignCurrency == "" {
			foreignCurrency = "USD"
		}
		params.ForeignAmountCents = &foreignCents
		params.ForeignCurrency = &foreignCurrency
	}
	if req.ExchangeRate != nil {
		params.ExchangeRate = req.ExchangeRate
	}
	if req.AccountId != nil {
		params.AccountID = req.AccountId
	}

	return params
}

func toPgTimeOfDay(tod *pb.TimeOfDay) pgtype.Time {
	if tod == nil {
		return pgtype.Time{}
	}

	hours := tod.GetHours()
	minutes := tod.GetMinutes()
	hoursOutOfRange := hours < 0 || hours > 23
	minutesOutOfRange := minutes < 0 || minutes > 59
	hasInvalidParts := hoursOutOfRange || minutesOutOfRange
	if hasInvalidParts {
		return pgtype.Time{}
	}

	microseconds := int64(hours*3600+minutes*60) * 1_000_000
	return pgtype.Time{Microseconds: microseconds, Valid: true}
}

func transactionToPb(tx *sqlc.Transaction) *pb.Transaction {
	proto := &pb.Transaction{
		Id:                  tx.ID,
		TxDate:              timestamppb.New(tx.TxDate),
		TxAmount:            centsToMoney(tx.TxAmountCents, tx.TxCurrency),
		Direction:           pb.TransactionDirection(tx.TxDirection),
		AccountId:           tx.AccountID,
		ExternalId:          tx.ExternalID,
		Description:         tx.TxDesc,
		CategoryId:          tx.CategoryID,
		CategoryManuallySet: tx.CategoryManuallySet,
		Merchant:            tx.Merchant,
		MerchantManuallySet: tx.MerchantManuallySet,
		UserNotes:           tx.UserNotes,
		CreatedAt:           timestamppb.New(tx.CreatedAt),
		UpdatedAt:           timestamppb.New(tx.UpdatedAt),
		SplitFromId:         tx.SplitFromID,
		Forgiven:            tx.Forgiven,
	}

	if tx.BalanceAfterCents != nil && tx.BalanceCurrency != nil {
		proto.BalanceAfter = centsToMoney(*tx.BalanceAfterCents, *tx.BalanceCurrency)
	}
	if tx.ForeignAmountCents != nil && tx.ForeignCurrency != nil {
		proto.ForeignAmount = centsToMoney(*tx.ForeignAmountCents, *tx.ForeignCurrency)
	}
	if tx.ExchangeRate != nil {
		proto.ExchangeRate = tx.ExchangeRate
	}

	return proto
}

func validateCreateParams(params sqlc.CreateTransactionParams) error {
	if params.AccountID <= 0 {
		return fmt.Errorf("account_id must be greater than zero: %w", ErrValidation)
	}

	if params.TxDate.IsZero() {
		return fmt.Errorf("tx_date is required: %w", ErrValidation)
	}

	if params.TxCurrency == "" {
		return fmt.Errorf("tx_currency is required: %w", ErrValidation)
	}

	if params.TxAmountCents == 0 {
		return fmt.Errorf("tx_amount cannot be zero: %w", ErrValidation)
	}

	switch params.TxDirection {
	case 1, 2:
	default:
		return fmt.Errorf("tx_direction must be 1 or 2: %w", ErrValidation)
	}

	return nil
}

func validateUpdateRequest(req *pb.UpdateTransactionRequest) error {
	hasTxDate := req.TxDate != nil
	txDateInvalid := hasTxDate && !req.TxDate.IsValid()
	if txDateInvalid {
		return fmt.Errorf("tx_date is invalid: %w", ErrValidation)
	}

	hasTxAmount := req.TxAmount != nil
	missingTxAmountCurrency := hasTxAmount && req.TxAmount.GetCurrencyCode() == ""
	if missingTxAmountCurrency {
		return fmt.Errorf("tx_amount.currency_code is required when tx_amount is provided: %w", ErrValidation)
	}

	hasForeignAmount := req.ForeignAmount != nil
	missingForeignAmountCurrency := hasForeignAmount && req.ForeignAmount.GetCurrencyCode() == ""
	if missingForeignAmountCurrency {
		return fmt.Errorf("foreign_amount.currency_code is required when foreign_amount is provided: %w", ErrValidation)
	}

	return nil
}

func (s *txnSvc) processForeignCurrency(ctx context.Context, userID uuid.UUID, params *sqlc.CreateTransactionParams) (*sqlc.CreateTransactionParams, error) {
	account, err := s.queries.GetAccount(ctx, sqlc.GetAccountParams{UserID: userID, ID: params.AccountID})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch account: %w", err)
	}

	if params.TxCurrency == account.Account.AnchorCurrency {
		params.ForeignAmountCents = nil
		params.ForeignCurrency = nil
		params.ExchangeRate = nil
		return params, nil
	}

	foreignAmountCents := params.TxAmountCents
	foreignCurrency := params.TxCurrency

	rate, err := s.exchangeClient.GetExchangeRate(foreignCurrency, account.Account.AnchorCurrency, &params.TxDate)
	if err != nil {
		return nil, fmt.Errorf("failed to get exchange rate from %s to %s: %w", foreignCurrency, account.Account.AnchorCurrency, err)
	}

	params.TxAmountCents = int64(float64(foreignAmountCents) * rate)
	params.TxCurrency = account.Account.AnchorCurrency
	params.ForeignAmountCents = &foreignAmountCents
	params.ForeignCurrency = &foreignCurrency
	params.ExchangeRate = &rate

	return params, nil
}

func (s *txnSvc) applyRulesToTransaction(ctx context.Context, userID uuid.UUID, txID int64) {
	tx, err := s.queries.GetTransaction(ctx, sqlc.GetTransactionParams{UserID: userID, ID: txID})
	if err != nil {
		s.log.Warn("failed to fetch transaction for rule application", "tx_id", txID, "error", err)
		return
	}

	bothFieldsManuallySet := tx.CategoryManuallySet && tx.MerchantManuallySet
	if bothFieldsManuallySet {
		return
	}

	account, err := s.queries.GetAccount(ctx, sqlc.GetAccountParams{UserID: userID, ID: tx.AccountID})
	if err != nil {
		s.log.Warn("failed to fetch account for rule application", "account_id", tx.AccountID, "error", err)
		return
	}

	result, err := s.ruleSvc.ApplyToTransaction(ctx, userID, &tx, &account)
	if err != nil {
		s.log.Warn("failed to apply rules", "tx_id", txID, "error", err)
		return
	}

	noRuleActions := result.CategoryID == nil && result.Merchant == nil
	if noRuleActions {
		return
	}

	updateParams := sqlc.UpdateTransactionParams{ID: txID, UserID: userID}
	if !tx.CategoryManuallySet && result.CategoryID != nil {
		updateParams.CategoryID = result.CategoryID
	}
	if !tx.MerchantManuallySet && result.Merchant != nil {
		updateParams.Merchant = result.Merchant
	}

	if err := s.queries.UpdateTransaction(ctx, updateParams); err != nil {
		s.log.Warn("failed to update transaction with rule results", "tx_id", txID, "error", err)
	}
}
