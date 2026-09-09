package service

import (
	"fmt"
	"strings"

	"nagomi-core/internal/db/sqlc"
	pb "nagomi-core/internal/gen/nagomi/v1"

	"github.com/google/uuid"
)

func parseUserID(id string) (uuid.UUID, error) {
	userID, err := uuid.Parse(id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid user_id: %w", err)
	}
	return userID, nil
}

func normalizeEmail(email string) string {
	return strings.ToLower(email)
}

func normalizeOptionalEmail(email *string) *string {
	if email == nil {
		return nil
	}
	normalized := normalizeEmail(*email)
	return &normalized
}

func buildCreateUserParams(req *pb.CreateUserRequest) (sqlc.CreateUserParams, error) {
	userID, err := parseUserID(req.GetId())
	if err != nil {
		return sqlc.CreateUserParams{}, err
	}

	return sqlc.CreateUserParams{
		ID:          userID,
		Email:       req.GetEmail(),
		DisplayName: req.DisplayName,
	}, nil
}

func buildUpdateUserParams(req *pb.UpdateUserRequest) (sqlc.UpdateUserParams, error) {
	userID, err := parseUserID(req.GetId())
	if err != nil {
		return sqlc.UpdateUserParams{}, err
	}

	return sqlc.UpdateUserParams{
		ID:              userID,
		Email:           req.Email,
		DisplayName:     req.DisplayName,
		PrimaryCurrency: req.PrimaryCurrency,
		Timezone:        req.Timezone,
	}, nil
}

func userToPb(u *sqlc.User) *pb.User {
	if u == nil {
		return nil
	}

	return &pb.User{
		Id:              u.ID.String(),
		Email:           u.Email,
		DisplayName:     u.DisplayName,
		PrimaryCurrency: u.PrimaryCurrency,
		Timezone:        u.Timezone,
		CreatedAt:       toProtoTimestamp(&u.CreatedAt),
		UpdatedAt:       toProtoTimestamp(&u.UpdatedAt),
	}
}
