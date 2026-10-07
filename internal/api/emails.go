package api

import (
	"context"

	pb "nagomi-core/internal/gen/nagomi/v1"

	"connectrpc.com/connect"
)

func (s *Server) ReportEmail(ctx context.Context, req *connect.Request[pb.ReportEmailRequest]) (*connect.Response[pb.ReportEmailResponse], error) {
	if err := requireInternal(ctx); err != nil {
		return nil, err
	}

	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	if err := s.services.Emails.Report(ctx, userID, req.Msg); err != nil {
		return nil, wrapErr(err)
	}

	return connect.NewResponse(&pb.ReportEmailResponse{}), nil
}

func (s *Server) ListRecentEmails(ctx context.Context, _ *connect.Request[pb.ListRecentEmailsRequest]) (*connect.Response[pb.ListRecentEmailsResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	emails, err := s.services.Emails.ListRecent(ctx, userID)
	if err != nil {
		return nil, wrapErr(err)
	}

	return connect.NewResponse(&pb.ListRecentEmailsResponse{Emails: emails}), nil
}
