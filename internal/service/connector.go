package service

import (
	"context"
	"fmt"
	"time"

	"nagomi-core/internal/crypto"
	"nagomi-core/internal/db/sqlc"

	"github.com/charmbracelet/log"
)

type ConnectorService interface {
	ListSyncJobs(ctx context.Context) ([]SyncJob, error)
	CompleteSyncJob(ctx context.Context, id int64, cursor *time.Time, status *string) error
}

// SyncJob is a decrypted view of a connected_accounts row. credentials
// are JSON bytes whose shape depends on provider. callers (nagomi-connector)
// are responsible for per-provider decoding.
type SyncJob struct {
	ID          int64
	UserID      string
	Provider    string
	Credentials []byte
	Cursor      *time.Time
}

type connSvc struct {
	queries *sqlc.Queries
	cipher  *crypto.Cipher
	log     *log.Logger
}

func newConnSvc(queries *sqlc.Queries, cipher *crypto.Cipher, logger *log.Logger) ConnectorService {
	return &connSvc{queries: queries, cipher: cipher, log: logger}
}

func (s *connSvc) ListSyncJobs(ctx context.Context) ([]SyncJob, error) {
	rows, err := s.queries.ListDueSyncJobs(ctx)
	if err != nil {
		return nil, fmt.Errorf("ConnectorService.ListSyncJobs: %w", err)
	}

	jobs := make([]SyncJob, 0, len(rows))
	for _, r := range rows {
		plaintext, err := s.cipher.Decrypt(r.Credentials)
		if err != nil {
			s.log.Error("failed to decrypt connected_account credentials",
				"id", r.ID, "user_id", r.UserID, "provider", r.Provider, "err", err)
			continue
		}
		jobs = append(jobs, SyncJob{
			ID:          r.ID,
			UserID:      r.UserID.String(),
			Provider:    r.Provider,
			Credentials: plaintext,
			Cursor:      r.SyncCursor,
		})
	}
	return jobs, nil
}

func (s *connSvc) CompleteSyncJob(ctx context.Context, id int64, cursor *time.Time, status *string) error {
	if status != nil {
		switch *status {
		case "active", "disabled", "broken":
		default:
			return fmt.Errorf("ConnectorService.CompleteSyncJob: invalid status %q", *status)
		}
	}

	if err := s.queries.CompleteSyncJob(ctx, sqlc.CompleteSyncJobParams{
		ID:         id,
		SyncCursor: cursor,
		Status:     status,
	}); err != nil {
		return fmt.Errorf("ConnectorService.CompleteSyncJob: %w", err)
	}
	return nil
}
