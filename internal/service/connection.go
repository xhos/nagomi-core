package service

import (
	"context"
	"fmt"
	"time"

	"nagomi-core/internal/crypto"
	"nagomi-core/internal/db/sqlc"

	"github.com/google/uuid"
)

type ConnectionService interface {
	List(ctx context.Context, userID uuid.UUID) ([]Connection, error)
	Create(ctx context.Context, userID uuid.UUID, provider string, credentialsJSON []byte, syncIntervalMinutes *int32) (int64, error)
	Delete(ctx context.Context, userID uuid.UUID, id int64) error
	TriggerSync(ctx context.Context, userID uuid.UUID, id int64) error
	SetSyncInterval(ctx context.Context, userID uuid.UUID, id int64, minutes *int32) error
}

type Connection struct {
	ID                  int64
	Provider            string
	Status              string
	LastSynced          *time.Time
	SyncIntervalMinutes *int32
	NextRunAt           *time.Time
	CreatedAt           time.Time
}

type connectionSvc struct {
	queries *sqlc.Queries
	cipher  *crypto.Cipher
}

func newConnectionSvc(queries *sqlc.Queries, cipher *crypto.Cipher) ConnectionService {
	return &connectionSvc{queries: queries, cipher: cipher}
}

func (s *connectionSvc) List(ctx context.Context, userID uuid.UUID) ([]Connection, error) {
	rows, err := s.queries.ListConnectionsForUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("ConnectionService.List: %w", err)
	}
	out := make([]Connection, len(rows))
	for i, r := range rows {
		out[i] = Connection{
			ID:                  r.ID,
			Provider:            r.Provider,
			Status:              r.Status,
			LastSynced:          r.SyncCursor,
			SyncIntervalMinutes: r.SyncIntervalMinutes,
			NextRunAt:           r.NextRunAt,
			CreatedAt:           r.CreatedAt,
		}
	}
	return out, nil
}

func (s *connectionSvc) Create(ctx context.Context, userID uuid.UUID, provider string, credentialsJSON []byte, syncIntervalMinutes *int32) (int64, error) {
	if syncIntervalMinutes != nil && *syncIntervalMinutes <= 0 {
		return 0, fmt.Errorf("sync interval must be positive: %w", ErrValidation)
	}
	switch provider {
	case "wise", "snaptrade":
	default:
		return 0, fmt.Errorf("ConnectionService.Create: unknown provider %q", provider)
	}

	ciphertext, err := s.cipher.Encrypt(credentialsJSON)
	if err != nil {
		return 0, fmt.Errorf("ConnectionService.Create: encrypt: %w", err)
	}

	row, err := s.queries.CreateConnectedAccount(ctx, sqlc.CreateConnectedAccountParams{
		UserID:              userID,
		Provider:            provider,
		Credentials:         ciphertext,
		SyncIntervalMinutes: syncIntervalMinutes,
	})
	if err != nil {
		return 0, fmt.Errorf("ConnectionService.Create: insert: %w", err)
	}
	return row.ID, nil
}

func (s *connectionSvc) Delete(ctx context.Context, userID uuid.UUID, id int64) error {
	affected, err := s.queries.DeleteConnectedAccount(ctx, sqlc.DeleteConnectedAccountParams{
		ID:     id,
		UserID: userID,
	})
	if err != nil {
		return fmt.Errorf("ConnectionService.Delete: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("ConnectionService.Delete: not found")
	}
	return nil
}

func (s *connectionSvc) TriggerSync(ctx context.Context, userID uuid.UUID, id int64) error {
	affected, err := s.queries.TriggerSync(ctx, sqlc.TriggerSyncParams{
		ID:     id,
		UserID: userID,
	})
	if err != nil {
		return fmt.Errorf("ConnectionService.TriggerSync: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("ConnectionService.TriggerSync: not found")
	}
	return nil
}

func (s *connectionSvc) SetSyncInterval(ctx context.Context, userID uuid.UUID, id int64, minutes *int32) error {
	if minutes != nil && *minutes <= 0 {
		return fmt.Errorf("sync interval must be positive: %w", ErrValidation)
	}
	affected, err := s.queries.SetSyncInterval(ctx, sqlc.SetSyncIntervalParams{
		ID:                  id,
		UserID:              userID,
		SyncIntervalMinutes: minutes,
	})
	if err != nil {
		return fmt.Errorf("ConnectionService.SetSyncInterval: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("ConnectionService.SetSyncInterval: not found")
	}
	return nil
}
