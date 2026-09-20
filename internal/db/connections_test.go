package db

import (
	"context"
	"nagomi-core/internal/db/sqlc"
	"testing"
	"time"
)

func TestSyncSchedulingAndFailureCursor(t *testing.T) {
	tdb := SetupTestDB(t)
	ctx := context.Background()
	user := tdb.CreateTestUser(ctx)
	weekly := int32(10080)
	c, err := tdb.CreateConnectedAccount(ctx, sqlc.CreateConnectedAccountParams{UserID: user, Provider: "wise", Credentials: []byte("test"), SyncIntervalMinutes: &weekly})
	if err != nil {
		t.Fatal(err)
	}
	cursor := time.Now().Add(-time.Hour).Truncate(time.Microsecond)
	if err := tdb.CompleteSyncJob(ctx, sqlc.CompleteSyncJobParams{ID: c.ID, SyncCursor: &cursor}); err != nil {
		t.Fatal(err)
	}
	broken := "broken"
	if err := tdb.CompleteSyncJob(ctx, sqlc.CompleteSyncJobParams{ID: c.ID, Status: &broken}); err != nil {
		t.Fatal(err)
	}
	c, err = tdb.GetConnectedAccount(ctx, c.ID)
	if err != nil || c.SyncCursor == nil || !c.SyncCursor.Equal(cursor) || c.Status != "broken" {
		t.Fatalf("failed sync advanced cursor: %+v %v", c, err)
	}
	if c.NextRunAt == nil || time.Until(*c.NextRunAt) < 6*24*time.Hour {
		t.Fatal("weekly failure should retain scheduled retry")
	}
	if _, err := tdb.SetSyncInterval(ctx, sqlc.SetSyncIntervalParams{ID: c.ID, UserID: user}); err != nil {
		t.Fatal(err)
	}
	c, err = tdb.GetConnectedAccount(ctx, c.ID)
	if err != nil || c.NextRunAt != nil {
		t.Fatal("manual mode must clear pending scheduled run")
	}
	if _, err := tdb.TriggerSync(ctx, sqlc.TriggerSyncParams{ID: c.ID, UserID: user}); err != nil {
		t.Fatal(err)
	}
	c, err = tdb.GetConnectedAccount(ctx, c.ID)
	if err != nil || c.Status != "active" || c.NextRunAt == nil {
		t.Fatal("manual retry should reactivate a failed connection")
	}
	if _, err := tdb.SetSyncInterval(ctx, sqlc.SetSyncIntervalParams{ID: c.ID, UserID: user, SyncIntervalMinutes: &weekly}); err != nil {
		t.Fatal(err)
	}
	c, err = tdb.GetConnectedAccount(ctx, c.ID)
	if err != nil || c.NextRunAt == nil || time.Until(*c.NextRunAt) < 6*24*time.Hour {
		t.Fatal("changing schedule must recalculate next run")
	}
}
