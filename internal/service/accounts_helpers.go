package service

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"nagomi-core/internal/db/sqlc"
	pb "nagomi-core/internal/gen/nagomi/v1"

	"github.com/google/uuid"
	"google.golang.org/genproto/googleapis/type/date"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func buildCreateAccountParams(req *pb.CreateAccountRequest) (sqlc.CreateAccountParams, error) {
	userID, err := uuid.Parse(req.GetUserId())
	if err != nil {
		return sqlc.CreateAccountParams{}, fmt.Errorf("invalid user_id: %w", err)
	}

	anchorBalance := req.GetAnchorBalance()
	return sqlc.CreateAccountParams{
		OwnerID:            userID,
		Name:               req.GetName(),
		Bank:               req.GetBank(),
		AccountType:        int16(req.GetType()),
		FriendlyName:       req.FriendlyName,
		AnchorBalanceCents: moneyToCents(anchorBalance),
		AnchorCurrency:     anchorBalance.GetCurrencyCode(),
		MainCurrency:       req.GetMainCurrency(),
		Color:              req.Color,
		StatementDriven:    req.GetStatementDriven(),
	}, nil
}

func buildUpdateAccountParams(userID uuid.UUID, req *pb.UpdateAccountRequest) sqlc.UpdateAccountParams {
	params := sqlc.UpdateAccountParams{ID: req.GetId(), UserID: userID}

	if req.Name != nil {
		params.Name = req.Name
	}
	if req.Bank != nil {
		params.Bank = req.Bank
	}
	if req.AccountType != nil {
		accountType := int16(*req.AccountType)
		params.AccountType = &accountType
	}
	if req.FriendlyName != nil {
		params.FriendlyName = req.FriendlyName
	}
	if req.AnchorDate != nil {
		anchorDate := req.AnchorDate.AsTime()
		params.AnchorDate = &anchorDate
	}
	if req.AnchorBalance != nil {
		cents := moneyToCents(req.AnchorBalance)
		params.AnchorBalanceCents = &cents
		if req.AnchorBalance.CurrencyCode != "" {
			currency := req.AnchorBalance.CurrencyCode
			params.AnchorCurrency = &currency
		}
	}
	if req.MainCurrency != nil {
		params.MainCurrency = req.MainCurrency
	}
	if req.Color != nil {
		params.Color = req.Color
	}
	// TODO: connector accounts must never be statement-driven, but core can't refuse
	// it: they're plain accounts the connector creates and finds by alias
	if req.StatementDriven != nil {
		params.StatementDriven = req.StatementDriven
	}

	masked := req.GetUpdateMask().GetPaths()
	if req.StatementsStart != nil {
		start := dateToUTC(req.StatementsStart)
		params.StatementsStart = &start
	} else if slices.Contains(masked, "statements_start") {
		params.ClearStatementsStart = true
	}
	if req.StatementReleaseDay != nil {
		day := int16(req.GetStatementReleaseDay())
		params.StatementReleaseDay = &day
	} else if slices.Contains(masked, "statement_release_day") {
		params.ClearStatementReleaseDay = true
	}
	if req.ClosedAt != nil {
		closed := dateToUTC(req.ClosedAt)
		params.ClosedAt = &closed
	} else if slices.Contains(masked, "closed_at") {
		params.ClearClosedAt = true
	}

	return params
}

func normalizeAlias(alias string) (string, error) {
	cleanAlias := strings.TrimSpace(alias)
	if cleanAlias == "" {
		return "", fmt.Errorf("%w: alias cannot be empty", ErrValidation)
	}
	return cleanAlias, nil
}

func normalizeAliases(aliases []string) ([]string, error) {
	seen := make(map[string]struct{}, len(aliases))
	cleaned := make([]string, 0, len(aliases))
	for _, alias := range aliases {
		cleanAlias, err := normalizeAlias(alias)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[cleanAlias]; exists {
			continue
		}
		seen[cleanAlias] = struct{}{}
		cleaned = append(cleaned, cleanAlias)
	}
	return cleaned, nil
}

func buildMergedAliases(primaryAliases []string, secondaryName string, secondaryAliases []string) []string {
	seen := make(map[string]struct{}, len(primaryAliases)+len(secondaryAliases)+1)
	merged := make([]string, 0, len(primaryAliases)+len(secondaryAliases)+1)

	for _, alias := range primaryAliases {
		if _, exists := seen[alias]; exists {
			continue
		}
		seen[alias] = struct{}{}
		merged = append(merged, alias)
	}

	candidates := append([]string{secondaryName}, secondaryAliases...)
	for _, candidate := range candidates {
		cleanCandidate := strings.TrimSpace(candidate)
		if cleanCandidate == "" {
			continue
		}
		if _, exists := seen[cleanCandidate]; exists {
			continue
		}
		seen[cleanCandidate] = struct{}{}
		merged = append(merged, cleanCandidate)
	}

	return merged
}

func accountRowToPb(a sqlc.Account, balanceCents int64, balanceCurrency string) *pb.Account {
	account := &pb.Account{
		Id:              a.ID,
		OwnerId:         a.OwnerID.String(),
		Name:            a.Name,
		Bank:            a.Bank,
		Type:            pb.AccountType(a.AccountType),
		FriendlyName:    a.FriendlyName,
		AnchorDate:      timestamppb.New(a.AnchorDate),
		AnchorBalance:   centsToMoney(a.AnchorBalanceCents, a.AnchorCurrency),
		MainCurrency:    a.MainCurrency,
		Color:           a.Color,
		Aliases:         a.Aliases,
		CreatedAt:       timestamppb.New(a.CreatedAt),
		UpdatedAt:       timestamppb.New(a.UpdatedAt),
		Balance:         centsToMoney(balanceCents, balanceCurrency),
		StatementDriven: a.StatementDriven,
		StatementsStart: optionalDate(a.StatementsStart),
		ClosedAt:        optionalDate(a.ClosedAt),
	}
	if a.StatementReleaseDay != nil {
		day := int32(*a.StatementReleaseDay)
		account.StatementReleaseDay = &day
	}
	return account
}

func optionalDate(t *time.Time) *date.Date {
	if t == nil {
		return nil
	}
	return timeToDate(*t)
}
