package service

import (
	"strings"
	"testing"

	"nagomi-core/internal/db/sqlc"
)

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
