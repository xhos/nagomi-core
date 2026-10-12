package service

import (
	"context"
	"errors"
	"slices"
	"testing"

	"nagomi-core/internal/db"

	"github.com/charmbracelet/log"
)

func TestCategoryRename(t *testing.T) {
	tdb := db.SetupTestDB(t)
	ctx := context.Background()
	user := tdb.CreateTestUser(ctx)
	svc := newCatSvc(tdb.Pool(), tdb.Queries, log.Default())

	create := func(slug string) int64 {
		c, err := svc.Create(ctx, user, slug, "#abcdef")
		if err != nil {
			t.Fatal(err)
		}
		return c.Id
	}
	slugs := func() []string {
		cats, err := svc.List(ctx, user)
		if err != nil {
			t.Fatal(err)
		}
		out := make([]string, len(cats))
		for i, c := range cats {
			out[i] = c.Slug
		}
		return out
	}
	expect := func(want ...string) {
		t.Helper()
		if got := slugs(); !slices.Equal(got, want) {
			t.Fatalf("slugs = %v, want %v", got, want)
		}
	}
	ptr := func(s string) *string { return &s }

	coffee := create("food.coffee")
	create("food_x")
	food, err := svc.GetBySlug(ctx, user, "food")
	if err != nil {
		t.Fatal(err)
	}

	// renames keep the id, children follow, and "food_x" isn't matched as a child
	if err := svc.Update(ctx, user, food.Id, ptr("eating"), nil); err != nil {
		t.Fatal(err)
	}
	expect("eating", "eating.coffee", "food_x")

	// moving under a new parent creates it
	if err := svc.Update(ctx, user, coffee, ptr("drinks.coffee"), nil); err != nil {
		t.Fatal(err)
	}
	expect("drinks", "drinks.coffee", "eating", "food_x")

	// color-only updates keep the slug
	if err := svc.Update(ctx, user, coffee, nil, ptr("#123456")); err != nil {
		t.Fatal(err)
	}
	if c, _ := svc.Get(ctx, user, coffee); c.Slug != "drinks.coffee" || c.Color != "#123456" {
		t.Fatalf("got %s %s", c.Slug, c.Color)
	}

	// a clash rolls back the whole rename, including the color
	create("eating.tea")
	drinks, _ := svc.GetBySlug(ctx, user, "drinks")
	if err := svc.Update(ctx, user, drinks.Id, ptr("eating"), ptr("#000000")); err == nil {
		t.Fatal("expected duplicate error")
	}
	expect("drinks", "drinks.coffee", "eating", "eating.tea", "food_x")
	if c, _ := svc.Get(ctx, user, drinks.Id); c.Color == "#000000" {
		t.Fatal("color changed despite failed rename")
	}

	if err := svc.Update(ctx, user, drinks.Id, ptr("drinks.hot"), nil); !errors.Is(err, ErrValidation) {
		t.Fatalf("moving under itself: got %v, want ErrValidation", err)
	}
}
