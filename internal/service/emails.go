package service

import (
	"context"

	pb "nagomi-core/internal/gen/nagomi/v1"
	"nagomi-core/internal/db/sqlc"

	"github.com/charmbracelet/log"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// recentEmailsKept is how many received emails each user keeps; older ones are deleted
const recentEmailsKept = 3

type EmailService interface {
	Report(ctx context.Context, userID uuid.UUID, req *pb.ReportEmailRequest) error
	ListRecent(ctx context.Context, userID uuid.UUID) ([]*pb.ReceivedEmail, error)
}

type emailSvc struct {
	queries *sqlc.Queries
	log     *log.Logger
}

func newEmailSvc(queries *sqlc.Queries, logger *log.Logger) EmailService {
	return &emailSvc{queries: queries, log: logger}
}

func (s *emailSvc) Report(ctx context.Context, userID uuid.UUID, req *pb.ReportEmailRequest) error {
	body := req.Body
	if req.Outcome == pb.EmailOutcome_EMAIL_OUTCOME_IMPORTED {
		body = nil
	}

	err := s.queries.InsertReceivedEmail(ctx, sqlc.InsertReceivedEmailParams{
		UserID:        userID,
		Sender:        req.From,
		Subject:       req.Subject,
		Outcome:       req.Outcome,
		TransactionID: req.TransactionId,
		Error:         req.Error,
		Body:          body,
	})
	if err != nil {
		return wrapErr("EmailService.Report", err)
	}

	// a failed trim only leaves an extra row until the next email
	if err := s.queries.TrimReceivedEmails(ctx, sqlc.TrimReceivedEmailsParams{UserID: userID, Keep: recentEmailsKept}); err != nil {
		s.log.Warn("failed to trim received emails", "user_id", userID, "err", err)
	}
	return nil
}

func (s *emailSvc) ListRecent(ctx context.Context, userID uuid.UUID) ([]*pb.ReceivedEmail, error) {
	rows, err := s.queries.ListReceivedEmails(ctx, userID)
	if err != nil {
		return nil, wrapErr("EmailService.ListRecent", err)
	}

	out := make([]*pb.ReceivedEmail, len(rows))
	for i, r := range rows {
		out[i] = &pb.ReceivedEmail{
			Id:            r.ID,
			ReceivedAt:    timestamppb.New(r.ReceivedAt),
			From:          r.Sender,
			Subject:       r.Subject,
			Outcome:       r.Outcome,
			TransactionId: r.TransactionID,
			Error:         r.Error,
			Body:          r.Body,
		}
	}
	return out, nil
}
