package service

import (
	"strings"
	"testing"
	"time"

	"nagomi-core/internal/db/sqlc"
	pb "nagomi-core/internal/gen/nagomi/v1"

	"github.com/google/uuid"
)

func TestParseUserID_ValidAndInvalid(t *testing.T) {
	id := uuid.New().String()
	parsed, err := parseUserID(id)
	if err != nil {
		t.Fatalf("expected valid user id, got error: %v", err)
	}
	if parsed.String() != id {
		t.Fatalf("expected parsed id %s, got %s", id, parsed.String())
	}

	_, err = parseUserID("not-a-uuid")
	if err == nil {
		t.Fatalf("expected error for invalid user id")
	}
}

func TestNormalizeEmailHelpers(t *testing.T) {
	if normalizeEmail("TesT@Example.COM") != "test@example.com" {
		t.Fatalf("expected lowercase normalization")
	}

	if normalizeOptionalEmail(nil) != nil {
		t.Fatalf("expected nil optional email to remain nil")
	}

	email := "UPPER@MAIL.COM"
	normalized := normalizeOptionalEmail(&email)
	if normalized == nil || *normalized != "upper@mail.com" {
		t.Fatalf("expected normalized optional email, got %v", normalized)
	}
}

func TestBuildCreateAndUpdateUserParams(t *testing.T) {
	id := uuid.New().String()
	displayName := "Test User"
	email := "User@Example.com"
	currency := "USD"
	timezone := "America/Toronto"

	createParams, err := buildCreateUserParams(&pb.CreateUserRequest{
		Id:          id,
		Email:       email,
		DisplayName: &displayName,
	})
	if err != nil {
		t.Fatalf("expected valid create params, got error: %v", err)
	}
	if createParams.ID.String() != id || createParams.Email != email {
		t.Fatalf("expected id/email mapped, got id=%s email=%s", createParams.ID, createParams.Email)
	}

	updateParams, err := buildUpdateUserParams(&pb.UpdateUserRequest{
		Id:              id,
		Email:           &email,
		DisplayName:     &displayName,
		PrimaryCurrency: &currency,
		Timezone:        &timezone,
	})
	if err != nil {
		t.Fatalf("expected valid update params, got error: %v", err)
	}
	if updateParams.ID.String() != id {
		t.Fatalf("expected mapped id %s, got %s", id, updateParams.ID)
	}
	if updateParams.Email == nil || *updateParams.Email != email {
		t.Fatalf("expected mapped email pointer, got %v", updateParams.Email)
	}
	if updateParams.PrimaryCurrency == nil || *updateParams.PrimaryCurrency != currency {
		t.Fatalf("expected mapped primary_currency, got %v", updateParams.PrimaryCurrency)
	}

	_, err = buildCreateUserParams(&pb.CreateUserRequest{Id: "bad-id"})
	if err == nil {
		t.Fatalf("expected create params error for invalid id")
	}
	_, err = buildUpdateUserParams(&pb.UpdateUserRequest{Id: "bad-id"})
	if err == nil {
		t.Fatalf("expected update params error for invalid id")
	}
}

func TestUserToPb_MapsAllFields(t *testing.T) {
	now := time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)
	displayName := "Display"
	userID := uuid.New()

	user := &sqlc.User{
		ID:              userID,
		Email:           "user@example.com",
		DisplayName:     &displayName,
		PrimaryCurrency: "CAD",
		Timezone:        "America/Toronto",
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	pbUser := userToPb(user)
	if pbUser == nil {
		t.Fatalf("expected non-nil pb user")
	}
	if pbUser.Id != userID.String() || pbUser.Email != user.Email {
		t.Fatalf("expected id/email mapped, got id=%s email=%s", pbUser.Id, pbUser.Email)
	}
	if pbUser.DisplayName == nil || *pbUser.DisplayName != displayName {
		t.Fatalf("expected display name mapped, got %v", pbUser.DisplayName)
	}
	if pbUser.CreatedAt == nil || pbUser.UpdatedAt == nil {
		t.Fatalf("expected created_at and updated_at timestamps")
	}

	if userToPb(nil) != nil {
		t.Fatalf("expected nil input to return nil")
	}

	if !strings.Contains(pbUser.Timezone, "Toronto") {
		t.Fatalf("expected timezone mapped, got %s", pbUser.Timezone)
	}
}
