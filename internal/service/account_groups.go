package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"nagomi-core/internal/db/sqlc"
	pb "nagomi-core/internal/gen/nagomi/v1"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// groups only shape how accounts are shown and filtered. nothing that acts on an
// account (rules, imports, transfers) looks at them

func (s *acctSvc) ListGroups(ctx context.Context, userID uuid.UUID) ([]*pb.AccountGroup, error) {
	rows, err := s.queries.ListAccountGroups(ctx, userID)
	if err != nil {
		return nil, wrapErr("AccountService.ListGroups", err)
	}
	groups := make([]*pb.AccountGroup, len(rows))
	for i, row := range rows {
		groups[i] = accountGroupToPb(row)
	}
	return groups, nil
}

func (s *acctSvc) CreateGroup(ctx context.Context, userID uuid.UUID, req *pb.CreateAccountGroupRequest) (*pb.AccountGroup, error) {
	name, err := groupName(req.GetName())
	if err != nil {
		return nil, wrapErr("AccountService.CreateGroup", err)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, wrapErr("AccountService.CreateGroup.Begin", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.queries.WithTx(tx)

	group, err := q.CreateAccountGroup(ctx, sqlc.CreateAccountGroupParams{
		UserID: userID,
		Name:   name,
		Color:  req.Color,
	})
	if err != nil {
		return nil, wrapErr("AccountService.CreateGroup", duplicateName(err))
	}
	if len(req.GetAccountIds()) > 0 {
		if _, err := q.SetAccountsGroup(ctx, sqlc.SetAccountsGroupParams{
			GroupID:    &group.ID,
			UserID:     userID,
			AccountIds: req.GetAccountIds(),
		}); err != nil {
			return nil, wrapErr("AccountService.CreateGroup.SetAccounts", err)
		}
		// accounts moved out of another group may have left it empty
		if err := q.DeleteEmptyAccountGroups(ctx, userID); err != nil {
			return nil, wrapErr("AccountService.CreateGroup.Prune", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, wrapErr("AccountService.CreateGroup.Commit", err)
	}
	return accountGroupToPb(group), nil
}

func (s *acctSvc) UpdateGroup(ctx context.Context, userID uuid.UUID, req *pb.UpdateAccountGroupRequest) error {
	params := sqlc.UpdateAccountGroupParams{ID: req.GetId(), UserID: userID, Color: req.Color}
	if req.Name != nil {
		name, err := groupName(req.GetName())
		if err != nil {
			return wrapErr("AccountService.UpdateGroup", err)
		}
		params.Name = &name
	}
	affected, err := s.queries.UpdateAccountGroup(ctx, params)
	if err != nil {
		return wrapErr("AccountService.UpdateGroup", duplicateName(err))
	}
	if affected == 0 {
		return wrapErr("AccountService.UpdateGroup", pgx.ErrNoRows)
	}
	return nil
}

func (s *acctSvc) DeleteGroup(ctx context.Context, userID uuid.UUID, id int64) error {
	affected, err := s.queries.DeleteAccountGroup(ctx, sqlc.DeleteAccountGroupParams{ID: id, UserID: userID})
	if err != nil {
		return wrapErr("AccountService.DeleteGroup", err)
	}
	if affected == 0 {
		return wrapErr("AccountService.DeleteGroup", pgx.ErrNoRows)
	}
	return nil
}

func (s *acctSvc) SetGroup(ctx context.Context, userID uuid.UUID, accountIDs []int64, groupID *int64) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return wrapErr("AccountService.SetGroup.Begin", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.queries.WithTx(tx)

	affected, err := q.SetAccountsGroup(ctx, sqlc.SetAccountsGroupParams{
		GroupID:    groupID,
		UserID:     userID,
		AccountIds: accountIDs,
	})
	if err != nil {
		return wrapErr("AccountService.SetGroup", err)
	}
	// nothing moved: the group isn't the user's, or none of the accounts are
	if affected == 0 {
		return wrapErr("AccountService.SetGroup", pgx.ErrNoRows)
	}
	if err := q.DeleteEmptyAccountGroups(ctx, userID); err != nil {
		return wrapErr("AccountService.SetGroup.Prune", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return wrapErr("AccountService.SetGroup.Commit", err)
	}
	return nil
}

func groupName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", fmt.Errorf("group name is required: %w", ErrValidation)
	}
	return name, nil
}

func duplicateName(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return fmt.Errorf("a group with that name already exists: %w", ErrDuplicate)
	}
	return err
}

func accountGroupToPb(g sqlc.AccountGroup) *pb.AccountGroup {
	return &pb.AccountGroup{
		Id:        g.ID,
		Name:      g.Name,
		Color:     g.Color,
		CreatedAt: timestamppb.New(g.CreatedAt),
		UpdatedAt: timestamppb.New(g.UpdatedAt),
	}
}
