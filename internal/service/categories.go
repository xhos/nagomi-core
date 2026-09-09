package service

import (
	"context"

	"nagomi-core/internal/db/sqlc"
	pb "nagomi-core/internal/gen/nagomi/v1"

	"github.com/charmbracelet/log"
	"github.com/google/uuid"
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
	queries *sqlc.Queries
	log     *log.Logger
}

func newCatSvc(queries *sqlc.Queries, logger *log.Logger) CategoryService {
	return &catSvc{queries: queries, log: logger}
}

// ----- methods -----------------------------------------------------------------------------

func (s *catSvc) Create(ctx context.Context, userID uuid.UUID, slug, color string) (*pb.Category, error) {
	if err := s.ensureParentCategories(ctx, userID, slug); err != nil {
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
	if slug != nil {
		oldCategory, err := s.queries.GetCategory(ctx, sqlc.GetCategoryParams{ID: categoryID, UserID: userID})
		if err != nil {
			return wrapErr("CategoryService.Update", err)
		}

		slugIsChanging := shouldUpdateCategoryHierarchy(oldCategory.Slug, slug)
		if slugIsChanging {
			if err := s.ensureParentCategories(ctx, userID, *slug); err != nil {
				return wrapErr("CategoryService.Update", err)
			}

			_, err = s.queries.UpdateChildCategorySlugs(ctx, sqlc.UpdateChildCategorySlugsParams{
				UserID:        userID,
				OldSlugPrefix: oldCategory.Slug,
				NewSlugPrefix: *slug,
			})
			if err != nil {
				return wrapErr("CategoryService.Update", err)
			}
		}
	}

	err := s.queries.UpdateCategory(ctx, buildUpdateCategoryParams(userID, categoryID, slug, color))
	if err != nil {
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
