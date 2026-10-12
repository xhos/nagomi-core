package api

import (
	"context"
	"errors"

	"nagomi-core/internal/api/middleware"
	pb "nagomi-core/internal/gen/nagomi/v1"

	"connectrpc.com/connect"
)

// users are created on first sign-in and never deleted through the api
func (s *Server) CreateUser(ctx context.Context, req *connect.Request[pb.CreateUserRequest]) (*connect.Response[pb.CreateUserResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("users are created on sign-in"))
}

func (s *Server) GetUser(ctx context.Context, req *connect.Request[pb.GetUserRequest]) (*connect.Response[pb.GetUserResponse], error) {
	id, err := selfID(ctx, req.Msg.GetId())
	if err != nil {
		return nil, err
	}

	user, err := s.services.Users.Get(ctx, id)
	if err != nil {
		return nil, wrapErr(err)
	}

	return connect.NewResponse(&pb.GetUserResponse{User: user}), nil
}

func (s *Server) UpdateUser(ctx context.Context, req *connect.Request[pb.UpdateUserRequest]) (*connect.Response[pb.UpdateUserResponse], error) {
	id, err := selfID(ctx, req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	req.Msg.Id = id

	if err := s.services.Users.Update(ctx, req.Msg); err != nil {
		return nil, wrapErr(err)
	}

	return connect.NewResponse(&pb.UpdateUserResponse{}), nil
}

func (s *Server) DeleteUser(ctx context.Context, req *connect.Request[pb.DeleteUserRequest]) (*connect.Response[pb.DeleteUserResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("deleting users is not supported"))
}

// selfID picks the user a user rpc acts on: internal services name one, signed-in users only get themselves
func selfID(ctx context.Context, requested string) (string, error) {
	if internal, _ := ctx.Value(middleware.InternalAuthKey).(bool); internal {
		return requested, nil
	}

	userID, err := getUserID(ctx)
	if err != nil {
		return "", err
	}
	if requested != "" && requested != userID.String() {
		return "", connect.NewError(connect.CodePermissionDenied, errors.New("cannot access another user"))
	}

	return userID.String(), nil
}
