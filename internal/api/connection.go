package api

import (
	"context"

	pb "nagomi-core/internal/gen/nagomi/v1"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (s *Server) ListConnections(ctx context.Context, _ *connect.Request[pb.ListConnectionsRequest]) (*connect.Response[pb.ListConnectionsResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	conns, err := s.services.Connections.List(ctx, userID)
	if err != nil {
		return nil, wrapErr(err)
	}

	out := make([]*pb.Connection, len(conns))
	for i, c := range conns {
		pbc := &pb.Connection{
			Id:        c.ID,
			Provider:  c.Provider,
			Status:    c.Status,
			CreatedAt: timestamppb.New(c.CreatedAt),
		}
		if c.LastSynced != nil {
			pbc.LastSynced = timestamppb.New(*c.LastSynced)
		}
		if c.SyncIntervalMinutes != nil {
			pbc.SyncIntervalMinutes = c.SyncIntervalMinutes
		}
		if c.NextRunAt != nil {
			pbc.NextRunAt = timestamppb.New(*c.NextRunAt)
		}
		out[i] = pbc
	}
	return connect.NewResponse(&pb.ListConnectionsResponse{Connections: out}), nil
}

func (s *Server) CreateConnection(ctx context.Context, req *connect.Request[pb.CreateConnectionRequest]) (*connect.Response[pb.CreateConnectionResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	id, err := s.services.Connections.Create(ctx, userID, req.Msg.GetProvider(), []byte(req.Msg.GetCredentials()), req.Msg.SyncIntervalMinutes)
	if err != nil {
		return nil, wrapErr(err)
	}
	return connect.NewResponse(&pb.CreateConnectionResponse{Id: id}), nil
}

func (s *Server) DeleteConnection(ctx context.Context, req *connect.Request[pb.DeleteConnectionRequest]) (*connect.Response[pb.DeleteConnectionResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	if err := s.services.Connections.Delete(ctx, userID, req.Msg.GetId()); err != nil {
		return nil, wrapErr(err)
	}
	return connect.NewResponse(&pb.DeleteConnectionResponse{}), nil
}

func (s *Server) TriggerSync(ctx context.Context, req *connect.Request[pb.TriggerSyncRequest]) (*connect.Response[pb.TriggerSyncResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.services.Connections.TriggerSync(ctx, userID, req.Msg.GetId()); err != nil {
		return nil, wrapErr(err)
	}
	return connect.NewResponse(&pb.TriggerSyncResponse{}), nil
}

func (s *Server) SetSyncInterval(ctx context.Context, req *connect.Request[pb.SetSyncIntervalRequest]) (*connect.Response[pb.SetSyncIntervalResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.services.Connections.SetSyncInterval(ctx, userID, req.Msg.GetId(), req.Msg.SyncIntervalMinutes); err != nil {
		return nil, wrapErr(err)
	}
	return connect.NewResponse(&pb.SetSyncIntervalResponse{}), nil
}
