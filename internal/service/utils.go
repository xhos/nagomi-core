package service

import (
	"errors"
	"fmt"
	"time"

	"google.golang.org/genproto/googleapis/type/date"
	"google.golang.org/genproto/googleapis/type/money"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var (
	ErrValidation    = errors.New("validation failed")
	ErrUnimplemented = errors.New("unimplemented")
	ErrDuplicate     = errors.New("duplicate")
)

// DuplicateReceiptError is returned when an uploaded receipt image matches one already stored.
// The ExistingID field lets the API layer return the conflicting receipt's ID to the client.
type DuplicateReceiptError struct {
	ExistingID int64
}

func (e *DuplicateReceiptError) Error() string {
	return fmt.Sprintf("receipt already uploaded (id %d): %v", e.ExistingID, ErrDuplicate)
}

func (e *DuplicateReceiptError) Unwrap() error {
	return ErrDuplicate
}

func wrapErr(op string, err error) error {
	return fmt.Errorf("%s: %w", op, err)
}

func int32Ptr(i int32) *int32 {
	return &i
}

func toProtoTimestamp(t *time.Time) *timestamppb.Timestamp {
	if t == nil || t.IsZero() {
		return nil
	}
	return timestamppb.New(*t)
}

func fromProtoTimestamp(ts *timestamppb.Timestamp) time.Time {
	if ts == nil || !ts.IsValid() {
		return time.Time{}
	}
	return ts.AsTime()
}

func moneyToCents(m *money.Money) int64 {
	if m == nil {
		return 0
	}
	return m.Units*100 + int64(m.Nanos)/10_000_000
}

func centsToMoney(cents int64, currency string) *money.Money {
	return &money.Money{
		CurrencyCode: currency,
		Units:        cents / 100,
		Nanos:        int32((cents % 100) * 10_000_000),
	}
}

func dateToTime(d *date.Date) *time.Time {
	if d == nil {
		return nil
	}
	t := time.Date(int(d.Year), time.Month(d.Month), int(d.Day), 0, 0, 0, 0, time.UTC)
	return &t
}

func timeToDate(t time.Time) *date.Date {
	if t.IsZero() {
		return nil
	}
	return &date.Date{
		Year:  int32(t.Year()),
		Month: int32(t.Month()),
		Day:   int32(t.Day()),
	}
}
