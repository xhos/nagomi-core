package api

import (
	"context"
	"errors"
	"time"

	"nagomi-core/internal/api/middleware"
	"nagomi-core/internal/service"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/genproto/googleapis/type/date"
	"google.golang.org/genproto/googleapis/type/money"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func wrapErr(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, service.ErrDuplicate) {
		return status.Error(codes.AlreadyExists, err.Error())
	}
	if errors.Is(err, service.ErrValidation) {
		return status.Error(codes.InvalidArgument, err.Error())
	}
	if errors.Is(err, service.ErrUnimplemented) {
		return status.Error(codes.Unimplemented, err.Error())
	}

	return status.Errorf(codes.Internal, "internal error: %v", err)
}

func getUserID(ctx context.Context) (uuid.UUID, error) {
	userID, ok := ctx.Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		return uuid.Nil, connect.NewError(connect.CodeUnauthenticated, errors.New("user not authenticated or user_id not found"))
	}
	return userID, nil
}

func dateToTime(d *date.Date) *time.Time {
	if d == nil {
		return nil
	}
	t := time.Date(int(d.Year), time.Month(d.Month), int(d.Day), 0, 0, 0, 0, time.UTC)
	return &t
}

func centsToMoney(cents int64, currency string) *money.Money {
	return &money.Money{
		CurrencyCode: currency,
		Units:        cents / 100,
		Nanos:        int32((cents % 100) * 10_000_000),
	}
}

// TODO: should be gone?
func parseUUID(s string) (uuid.UUID, error) {
	if s == "" {
		return uuid.Nil, status.Error(codes.InvalidArgument, "uuid cannot be empty")
	}

	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, status.Errorf(codes.InvalidArgument, "invalid uuid: %v", err)
	}

	return id, nil
}
