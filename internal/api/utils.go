package api

import (
	"context"
	"errors"
	"fmt"
	"time"

	"nagomi-core/internal/api/middleware"
	"nagomi-core/internal/service"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"google.golang.org/genproto/googleapis/type/date"
	"google.golang.org/genproto/googleapis/type/money"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func wrapErr(err error) error {
	if err == nil {
		return nil
	}

	// connect errors, not grpc-go status errors: connect doesn't recognise the latter
	// and would send every one of them as code unknown
	if errors.Is(err, service.ErrDuplicate) {
		return connect.NewError(connect.CodeAlreadyExists, err)
	}
	if errors.Is(err, service.ErrValidation) {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}
	if errors.Is(err, service.ErrUnimplemented) {
		return connect.NewError(connect.CodeUnimplemented, err)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return connect.NewError(connect.CodeNotFound, errors.New("not found"))
	}

	return connect.NewError(connect.CodeInternal, fmt.Errorf("internal error: %w", err))
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
