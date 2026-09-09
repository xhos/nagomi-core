package api

import (
	"context"

	pb "nagomi-core/internal/gen/nagomi/v1"

	"connectrpc.com/connect"
)

func (s *Server) CreateCategory(ctx context.Context, req *connect.Request[pb.CreateCategoryRequest]) (*connect.Response[pb.CreateCategoryResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	cat, err := s.services.Categories.Create(ctx, userID, req.Msg.GetSlug(), req.Msg.GetColor())
	if err != nil {
		return nil, wrapErr(err)
	}

	return connect.NewResponse(&pb.CreateCategoryResponse{Category: cat}), nil
}

func (s *Server) GetCategory(ctx context.Context, req *connect.Request[pb.GetCategoryRequest]) (*connect.Response[pb.GetCategoryResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	cat, err := s.services.Categories.Get(ctx, userID, req.Msg.GetId())
	if err != nil {
		return nil, wrapErr(err)
	}

	return connect.NewResponse(&pb.GetCategoryResponse{Category: cat}), nil
}

func (s *Server) UpdateCategory(ctx context.Context, req *connect.Request[pb.UpdateCategoryRequest]) (*connect.Response[pb.UpdateCategoryResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	err = s.services.Categories.Update(ctx, userID, req.Msg.GetId(), req.Msg.Slug, req.Msg.Color)
	if err != nil {
		return nil, wrapErr(err)
	}

	return connect.NewResponse(&pb.UpdateCategoryResponse{}), nil
}

func (s *Server) DeleteCategory(ctx context.Context, req *connect.Request[pb.DeleteCategoryRequest]) (*connect.Response[pb.DeleteCategoryResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	id := req.Msg.GetId()

	affected, err := s.services.Categories.Delete(ctx, userID, id)
	if err != nil {
		return nil, wrapErr(err)
	}

	return connect.NewResponse(&pb.DeleteCategoryResponse{AffectedRows: affected}), nil
}

func (s *Server) ListCategories(ctx context.Context, req *connect.Request[pb.ListCategoriesRequest]) (*connect.Response[pb.ListCategoriesResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	cats, err := s.services.Categories.List(ctx, userID)
	if err != nil {
		return nil, wrapErr(err)
	}

	return connect.NewResponse(&pb.ListCategoriesResponse{Categories: cats, TotalCount: int64(len(cats))}), nil
}
