package service

import (
	"strings"
	"testing"

	"nagomi-core/internal/db/sqlc"

	"github.com/google/uuid"
)

func TestBuildUpdateCategoryParams_MapsFields(t *testing.T) {
	userID := uuid.New()
	slug := "food.groceries"
	color := "#123456"

	params := buildUpdateCategoryParams(userID, 9, &slug, &color)
	if params.ID != 9 || params.UserID != userID {
		t.Fatalf("expected id/user mapped, got id=%d user=%v", params.ID, params.UserID)
	}
	if params.Slug == nil || *params.Slug != slug {
		t.Fatalf("expected slug mapped, got %v", params.Slug)
	}
	if params.Color == nil || *params.Color != color {
		t.Fatalf("expected color mapped, got %v", params.Color)
	}
}

func TestShouldUpdateCategoryHierarchy(t *testing.T) {
	same := "food"
	different := "food.groceries"

	if shouldUpdateCategoryHierarchy("food", nil) {
		t.Fatalf("expected nil slug to skip hierarchy update")
	}
	if shouldUpdateCategoryHierarchy("food", &same) {
		t.Fatalf("expected same slug to skip hierarchy update")
	}
	if !shouldUpdateCategoryHierarchy("food", &different) {
		t.Fatalf("expected different slug to require hierarchy update")
	}
}

func TestCategoryParentSlugs(t *testing.T) {
	noParents := categoryParentSlugs("food")
	if len(noParents) != 0 {
		t.Fatalf("expected no parent slugs for single segment, got %v", noParents)
	}

	parents := categoryParentSlugs("food.groceries.organic")
	expected := []string{"food", "food.groceries"}
	if len(parents) != len(expected) {
		t.Fatalf("expected %d parent slugs, got %d (%v)", len(expected), len(parents), parents)
	}
	for i := range expected {
		if parents[i] != expected[i] {
			t.Fatalf("expected parents[%d]=%q, got %q", i, expected[i], parents[i])
		}
	}
}

func TestCategoryToPb_MapsFields(t *testing.T) {
	category := sqlc.Category{ID: 7, Slug: "food", Color: "#abcdef"}
	pbCategory := categoryToPb(&category)

	if pbCategory.Id != 7 || pbCategory.Slug != "food" || pbCategory.Color != "#abcdef" {
		t.Fatalf("expected category fields mapped, got %+v", pbCategory)
	}
}

func TestGenerateNiceHexColor_FormatAndCharset(t *testing.T) {
	color := generateNiceHexColor()
	if len(color) != 7 {
		t.Fatalf("expected color length 7, got %d (%q)", len(color), color)
	}
	if !strings.HasPrefix(color, "#") {
		t.Fatalf("expected color to start with #, got %q", color)
	}
	for i := 1; i < len(color); i++ {
		if !strings.ContainsRune("56789ab", rune(color[i])) {
			t.Fatalf("expected character %q in allowed charset, color=%q", color[i], color)
		}
	}
}
