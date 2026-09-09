package api

import (
	"context"
	"errors"
	"time"

	"nagomi-core/internal/api/middleware"
	pb "nagomi-core/internal/gen/nagomi/v1"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func requireInternal(ctx context.Context) error {
	if internal, _ := ctx.Value(middleware.InternalAuthKey).(bool); !internal {
		return connect.NewError(connect.CodePermissionDenied, errors.New("connector service requires internal auth"))
	}
	return nil
}

func (s *Server) ListSyncJobs(ctx context.Context, _ *connect.Request[pb.ListSyncJobsRequest]) (*connect.Response[pb.ListSyncJobsResponse], error) {
	if err := requireInternal(ctx); err != nil {
		return nil, err
	}

	jobs, err := s.services.Connector.ListSyncJobs(ctx)
	if err != nil {
		return nil, wrapErr(err)
	}

	out := make([]*pb.SyncJob, len(jobs))
	for i, j := range jobs {
		pbj := &pb.SyncJob{
			Id:          j.ID,
			UserId:      j.UserID,
			Provider:    j.Provider,
			Credentials: j.Credentials,
		}
		if j.Cursor != nil {
			pbj.Cursor = timestamppb.New(*j.Cursor)
		}
		out[i] = pbj
	}

	return connect.NewResponse(&pb.ListSyncJobsResponse{Jobs: out}), nil
}

func (s *Server) CompleteSyncJob(ctx context.Context, req *connect.Request[pb.CompleteSyncJobRequest]) (*connect.Response[pb.CompleteSyncJobResponse], error) {
	if err := requireInternal(ctx); err != nil {
		return nil, err
	}

	var cursorTime *time.Time
	if c := req.Msg.GetCursor(); c != nil {
		t := c.AsTime()
		cursorTime = &t
	}

	if err := s.services.Connector.CompleteSyncJob(ctx, req.Msg.GetId(), cursorTime, req.Msg.Status); err != nil {
		return nil, wrapErr(err)
	}
	return connect.NewResponse(&pb.CompleteSyncJobResponse{}), nil
}
