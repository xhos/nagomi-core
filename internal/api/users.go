package api

import (
	"context"

	pb "nagomi-core/internal/gen/nagomi/v1"

	"connectrpc.com/connect"
)

func (s *Server) CreateUser(ctx context.Context, req *connect.Request[pb.CreateUserRequest]) (*connect.Response[pb.CreateUserResponse], error) {
	user, err := s.services.Users.Create(ctx, req.Msg)
	if err != nil {
		return nil, wrapErr(err)
	}

	return connect.NewResponse(&pb.CreateUserResponse{User: user}), nil
}

func (s *Server) GetUser(ctx context.Context, req *connect.Request[pb.GetUserRequest]) (*connect.Response[pb.GetUserResponse], error) {
	user, err := s.services.Users.Get(ctx, req.Msg.GetId())
	if err != nil {
		return nil, wrapErr(err)
	}

	return connect.NewResponse(&pb.GetUserResponse{User: user}), nil
}

func (s *Server) UpdateUser(ctx context.Context, req *connect.Request[pb.UpdateUserRequest]) (*connect.Response[pb.UpdateUserResponse], error) {
	err := s.services.Users.Update(ctx, req.Msg)
	if err != nil {
		return nil, wrapErr(err)
	}

	return connect.NewResponse(&pb.UpdateUserResponse{}), nil
}

func (s *Server) DeleteUser(ctx context.Context, req *connect.Request[pb.DeleteUserRequest]) (*connect.Response[pb.DeleteUserResponse], error) {
	err := s.services.Users.Delete(ctx, req.Msg.GetId())
	if err != nil {
		return nil, wrapErr(err)
	}

	return connect.NewResponse(&pb.DeleteUserResponse{}), nil
}
