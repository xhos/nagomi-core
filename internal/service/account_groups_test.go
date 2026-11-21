package service

import (
	"context"
	"errors"
	"testing"

	"nagomi-core/internal/db"
	"nagomi-core/internal/db/sqlc"
	pb "nagomi-core/internal/gen/nagomi/v1"

	"github.com/charmbracelet/log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestAccountGroups(t *testing.T) {
	tdb := db.SetupTestDB(t)
	ctx := context.Background()
	user := tdb.CreateTestUser(ctx)
	other := tdb.CreateTestUser(ctx)
	svc := newAcctSvc(tdb.Pool(), tdb.Queries, log.Default(), nil)

	account := func(owner uuid.UUID, name string) int64 {
		return tdb.CreateTestAccount(ctx, sqlc.CreateAccountParams{
			OwnerID: owner, Name: name, Bank: "wise",
			AnchorCurrency: "CAD", MainCurrency: "CAD",
		}).ID
	}
	groupOf := func(id int64) *int64 {
		t.Helper()
		a, err := svc.Get(ctx, user, id)
		if err != nil {
			t.Fatal(err)
		}
		return a.GroupId
	}
	groups := func() []*pb.AccountGroup {
		t.Helper()
		gs, err := svc.ListGroups(ctx, user)
		if err != nil {
			t.Fatal(err)
		}
		return gs
	}

	cad, usd, eur := account(user, "wise cad"), account(user, "wise usd"), account(user, "wise eur")
	theirs := account(other, "theirs")

	wise, err := svc.CreateGroup(ctx, user, &pb.CreateAccountGroupRequest{
		Name: " wise ", AccountIds: []int64{cad, usd, theirs},
	})
	if err != nil {
		t.Fatal(err)
	}
	if wise.Name != "wise" || wise.Color != "#3b82f6" {
		t.Fatalf("group = %q %q, want trimmed name and default color", wise.Name, wise.Color)
	}
	if g := groupOf(cad); g == nil || *g != wise.Id {
		t.Fatalf("cad group = %v, want %d", g, wise.Id)
	}
	if g := groupOf(eur); g != nil {
		t.Fatalf("eur joined a group it wasn't given: %d", *g)
	}
	// another user's account can't be pulled into the group
	if a, _ := svc.Get(ctx, other, theirs); a.GroupId != nil {
		t.Fatal("another user's account was grouped")
	}

	if _, err := svc.CreateGroup(ctx, user, &pb.CreateAccountGroupRequest{Name: "wise"}); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("duplicate name: err = %v, want duplicate", err)
	}

	// the other user can't move accounts into this group
	if err := svc.SetGroup(ctx, other, []int64{theirs}, &wise.Id); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("foreign group: err = %v, want not found", err)
	}

	if err := svc.SetGroup(ctx, user, []int64{eur}, &wise.Id); err != nil {
		t.Fatal(err)
	}
	if g := groupOf(eur); g == nil || *g != wise.Id {
		t.Fatal("eur wasn't moved into the group")
	}

	// moving every account out deletes the group
	if err := svc.SetGroup(ctx, user, []int64{cad, usd, eur}, nil); err != nil {
		t.Fatal(err)
	}
	if gs := groups(); len(gs) != 0 {
		t.Fatalf("empty group survived: %v", gs)
	}

	// deleting a group keeps its accounts
	again, err := svc.CreateGroup(ctx, user, &pb.CreateAccountGroupRequest{Name: "wise", AccountIds: []int64{cad}})
	if err != nil {
		t.Fatal(err)
	}
	green := "#10b981"
	if err := svc.UpdateGroup(ctx, user, &pb.UpdateAccountGroupRequest{Id: again.Id, Color: &green}); err != nil {
		t.Fatal(err)
	}
	if gs := groups(); len(gs) != 1 || gs[0].Color != "#10b981" {
		t.Fatalf("groups after recolor = %v", gs)
	}
	if err := svc.DeleteGroup(ctx, user, again.Id); err != nil {
		t.Fatal(err)
	}
	if g := groupOf(cad); g != nil {
		t.Fatal("account still points at a deleted group")
	}
}
