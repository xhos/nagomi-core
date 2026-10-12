package service

import (
	"context"
	"fmt"
	"strings"

	"nagomi-core/internal/db/sqlc"
	pb "nagomi-core/internal/gen/nagomi/v1"

	"github.com/charmbracelet/log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ----- interface ---------------------------------------------------------------------------

type CategoryService interface {
	Create(ctx context.Context, userID uuid.UUID, slug, color string) (*pb.Category, error)
	Get(ctx context.Context, userID uuid.UUID, categoryID int64) (*pb.Category, error)
	GetBySlug(ctx context.Context, userID uuid.UUID, slug string) (*pb.Category, error)
	Update(ctx context.Context, userID uuid.UUID, categoryID int64, slug, color *string) error
	Delete(ctx context.Context, userID uuid.UUID, categoryID int64) (int64, error)
	List(ctx context.Context, userID uuid.UUID) ([]*pb.Category, error)
}

type catSvc struct {
	pool    *pgxpool.Pool
	queries *sqlc.Queries
	log     *log.Logger
}

func newCatSvc(pool *pgxpool.Pool, queries *sqlc.Queries, logger *log.Logger) CategoryService {
	return &catSvc{pool: pool, queries: queries, log: logger}
}

// ----- methods -----------------------------------------------------------------------------

func (s *catSvc) Create(ctx context.Context, userID uuid.UUID, slug, color string) (*pb.Category, error) {
	if err := ensureParentCategories(ctx, s.queries, userID, slug); err != nil {
		return nil, wrapErr("CategoryService.Create", err)
	}

	category, err := s.queries.CreateCategory(ctx, sqlc.CreateCategoryParams{
		UserID: userID,
		Slug:   slug,
		Color:  color,
	})
	if err != nil {
		return nil, wrapErr("CategoryService.Create", err)
	}

	return categoryToPb(&category), nil
}

func (s *catSvc) Get(ctx context.Context, userID uuid.UUID, categoryID int64) (*pb.Category, error) {
	category, err := s.queries.GetCategory(ctx, sqlc.GetCategoryParams{
		ID:     categoryID,
		UserID: userID,
	})
	if err != nil {
		return nil, wrapErr("CategoryService.Get", err)
	}

	return categoryToPb(&category), nil
}

func (s *catSvc) GetBySlug(ctx context.Context, userID uuid.UUID, slug string) (*pb.Category, error) {
	category, err := s.queries.GetCategoryBySlug(ctx, sqlc.GetCategoryBySlugParams{
		Slug:   slug,
		UserID: userID,
	})
	if err != nil {
		return nil, wrapErr("CategoryService.BySlug", err)
	}

	return categoryToPb(&category), nil
}

func (s *catSvc) Update(ctx context.Context, userID uuid.UUID, categoryID int64, slug, color *string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return wrapErr("CategoryService.Update", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.queries.WithTx(tx)

	if slug != nil {
		old, err := q.GetCategory(ctx, sqlc.GetCategoryParams{ID: categoryID, UserID: userID})
		if err != nil {
			return wrapErr("CategoryService.Update", err)
		}

		if *slug != old.Slug {
			if strings.HasPrefix(*slug, old.Slug+".") {
				return fmt.Errorf("CategoryService.Update: can't move %q under itself: %w", old.Slug, ErrValidation)
			}

			// children move with their parent, so "food.coffee" follows "food" -> "eating"
			_, err = q.RenameCategoryTree(ctx, sqlc.RenameCategoryTreeParams{
				UserID:  userID,
				OldSlug: old.Slug,
				NewSlug: *slug,
			})
			if err != nil {
				return wrapErr("CategoryService.Update", err)
			}

			if err := ensureParentCategories(ctx, q, userID, *slug); err != nil {
				return wrapErr("CategoryService.Update", err)
			}
		}
	}

	if color != nil {
		err := q.UpdateCategoryColor(ctx, sqlc.UpdateCategoryColorParams{ID: categoryID, UserID: userID, Color: *color})
		if err != nil {
			return wrapErr("CategoryService.Update", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return wrapErr("CategoryService.Update", err)
	}
	return nil
}

func (s *catSvc) Delete(ctx context.Context, userID uuid.UUID, categoryID int64) (int64, error) {
	category, err := s.queries.GetCategory(ctx, sqlc.GetCategoryParams{
		ID:     categoryID,
		UserID: userID,
	})
	if err != nil {
		return 0, wrapErr("CategoryService.Delete", err)
	}

	// cascade delete children like "food.groceries" when deleting "food"
	affected, err := s.queries.DeleteCategoriesBySlugPrefix(ctx, sqlc.DeleteCategoriesBySlugPrefixParams{
		UserID: userID,
		Slug:   category.Slug,
	})
	if err != nil {
		return 0, wrapErr("CategoryService.Delete", err)
	}

	return affected, nil
}

func (s *catSvc) List(ctx context.Context, userID uuid.UUID) ([]*pb.Category, error) {
	rows, err := s.queries.ListCategories(ctx, userID)
	if err != nil {
		return nil, wrapErr("CategoryService.List", err)
	}

	result := make([]*pb.Category, len(rows))
	for i := range rows {
		result[i] = categoryToPb(&rows[i])
	}

	return result, nil
}
