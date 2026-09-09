package service

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"strings"

	"nagomi-core/internal/db/sqlc"
	pb "nagomi-core/internal/gen/nagomi/v1"

	"github.com/google/uuid"
)

func buildUpdateCategoryParams(userID uuid.UUID, categoryID int64, slug, color *string) sqlc.UpdateCategoryParams {
	return sqlc.UpdateCategoryParams{
		ID:     categoryID,
		UserID: userID,
		Slug:   slug,
		Color:  color,
	}
}

func shouldUpdateCategoryHierarchy(oldSlug string, newSlug *string) bool {
	if newSlug == nil {
		return false
	}
	return oldSlug != *newSlug
}

func categoryParentSlugs(slug string) []string {
	parts := strings.Split(slug, ".")
	hasParents := len(parts) > 1
	if !hasParents {
		return nil
	}

	parents := make([]string, 0, len(parts)-1)
	for i := 1; i < len(parts); i++ {
		parents = append(parents, strings.Join(parts[:i], "."))
	}

	return parents
}

func categoryToPb(c *sqlc.Category) *pb.Category {
	return &pb.Category{
		Id:    c.ID,
		Slug:  c.Slug,
		Color: c.Color,
	}
}

func (s *catSvc) ensureParentCategories(ctx context.Context, userID uuid.UUID, slug string) error {
	for _, parentSlug := range categoryParentSlugs(slug) {
		_, err := s.queries.GetCategoryBySlug(ctx, sqlc.GetCategoryBySlugParams{Slug: parentSlug, UserID: userID})
		if err == nil {
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}

		color := generateNiceHexColor()
		_, err = s.queries.CreateCategoryIfNotExists(ctx, sqlc.CreateCategoryIfNotExistsParams{
			UserID: userID,
			Slug:   parentSlug,
			Color:  color,
		})
		if err != nil {
			return err
		}
	}

	return nil
}

func generateNiceHexColor() string {
	const niceHexChars = "56789ab"
	const colorLength = 6

	randomBytes := make([]byte, colorLength)
	_, _ = rand.Read(randomBytes)

	color := "#"
	for i := 0; i < colorLength; i++ {
		charIndex := int(randomBytes[i]) % len(niceHexChars)
		color += string(niceHexChars[charIndex])
	}

	return color
}
