package service

import (
	"context"

	"nagomi-core/internal/db/sqlc"
	pb "nagomi-core/internal/gen/nagomi/v1"

	"github.com/charmbracelet/log"
)

// ----- interface ---------------------------------------------------------------------------

type UserService interface {
	EnsureExists(ctx context.Context, id, email, name string) error
	Get(ctx context.Context, id string) (*pb.User, error)
	Update(ctx context.Context, req *pb.UpdateUserRequest) error
}

type userSvc struct {
	queries *sqlc.Queries
	log     *log.Logger
}

func newUserSvc(queries *sqlc.Queries, logger *log.Logger) UserService {
	return &userSvc{queries: queries, log: logger}
}

// ----- methods -----------------------------------------------------------------------------

func (s *userSvc) EnsureExists(ctx context.Context, id, email, name string) error {
	userID, err := parseUserID(id)
	if err != nil {
		return wrapErr("UserService.EnsureExists", err)
	}

	var displayName *string
	if name != "" {
		displayName = &name
	}

	_, err = s.queries.UpsertUser(ctx, sqlc.UpsertUserParams{
		ID:          userID,
		Email:       normalizeEmail(email),
		DisplayName: displayName,
	})
	if err != nil {
		return wrapErr("UserService.EnsureExists", err)
	}

	return nil
}

func (s *userSvc) Get(ctx context.Context, id string) (*pb.User, error) {
	userID, err := parseUserID(id)
	if err != nil {
		return nil, wrapErr("UserService.Get", err)
	}

	user, err := s.queries.GetUser(ctx, userID)
	if err != nil {
		return nil, wrapErr("UserService.Get", err)
	}

	return userToPb(&user), nil
}

func (s *userSvc) Update(ctx context.Context, req *pb.UpdateUserRequest) error {
	params, err := buildUpdateUserParams(req)
	if err != nil {
		return wrapErr("UserService.Update", err)
	}

	if params.Email != nil {
		params.Email = normalizeOptionalEmail(params.Email)
	}

	err = s.queries.UpdateUser(ctx, params)
	if err != nil {
		return wrapErr("UserService.Update", err)
	}

	return nil
}
