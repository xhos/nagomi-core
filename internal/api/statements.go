package api

import (
	"context"
	"errors"

	pb "nagomi-core/internal/gen/nagomi/v1"
	"nagomi-core/internal/service"

	"connectrpc.com/connect"
)

func (s *Server) PreviewStatementImport(ctx context.Context, req *connect.Request[pb.PreviewStatementImportRequest]) (*connect.Response[pb.PreviewStatementImportResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	preview, err := s.services.Statements.Preview(ctx, userID, req.Msg.GetPdfData(), req.Msg.GetFileName())
	if err != nil {
		var dupErr *service.DuplicateStatementError
		if errors.As(err, &dupErr) {
			connectErr := connect.NewError(connect.CodeAlreadyExists, errors.New("statement has already been imported"))
			if detail, detailErr := connect.NewErrorDetail(&pb.Statement{Id: dupErr.ExistingID}); detailErr == nil {
				connectErr.AddDetail(detail)
			}
			return nil, connectErr
		}
		return nil, wrapErr(err)
	}

	return connect.NewResponse(preview), nil
}

func (s *Server) PlanStatementImport(ctx context.Context, req *connect.Request[pb.PlanStatementImportRequest]) (*connect.Response[pb.PlanStatementImportResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	reconciliation, err := s.services.Statements.Plan(ctx, userID, req.Msg.GetStatementId(), req.Msg.GetAccountId())
	if err != nil {
		return nil, wrapErr(err)
	}

	return connect.NewResponse(&pb.PlanStatementImportResponse{Reconciliation: reconciliation}), nil
}

func (s *Server) CommitStatementImport(ctx context.Context, req *connect.Request[pb.CommitStatementImportRequest]) (*connect.Response[pb.CommitStatementImportResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	result, err := s.services.Statements.Commit(ctx, userID, req.Msg)
	if err != nil {
		return nil, wrapErr(err)
	}

	return connect.NewResponse(result), nil
}

func (s *Server) ListStatements(ctx context.Context, req *connect.Request[pb.ListStatementsRequest]) (*connect.Response[pb.ListStatementsResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	statements, err := s.services.Statements.List(ctx, userID, req.Msg)
	if err != nil {
		return nil, wrapErr(err)
	}

	return connect.NewResponse(&pb.ListStatementsResponse{Statements: statements}), nil
}

func (s *Server) GetStatement(ctx context.Context, req *connect.Request[pb.GetStatementRequest]) (*connect.Response[pb.GetStatementResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	statement, pdfData, err := s.services.Statements.Get(ctx, userID, req.Msg.GetId())
	if err != nil {
		return nil, wrapErr(err)
	}

	return connect.NewResponse(&pb.GetStatementResponse{Statement: statement, PdfData: pdfData}), nil
}

func (s *Server) DeleteStatement(ctx context.Context, req *connect.Request[pb.DeleteStatementRequest]) (*connect.Response[pb.DeleteStatementResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	deleted, err := s.services.Statements.Delete(ctx, userID, req.Msg.GetId(), req.Msg.GetDeleteTransactions())
	if err != nil {
		return nil, wrapErr(err)
	}

	return connect.NewResponse(&pb.DeleteStatementResponse{DeletedTransactions: deleted}), nil
}

func (s *Server) ReparseStatement(ctx context.Context, req *connect.Request[pb.ReparseStatementRequest]) (*connect.Response[pb.ReparseStatementResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	result, err := s.services.Statements.Reparse(ctx, userID, req.Msg.GetId(), req.Msg.GetApply())
	if err != nil {
		return nil, wrapErr(err)
	}

	return connect.NewResponse(result), nil
}

func (s *Server) GetStatementCoverage(ctx context.Context, req *connect.Request[pb.GetStatementCoverageRequest]) (*connect.Response[pb.GetStatementCoverageResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	periods, err := s.services.Statements.Coverage(ctx, userID, req.Msg.GetAccountId())
	if err != nil {
		return nil, wrapErr(err)
	}

	return connect.NewResponse(&pb.GetStatementCoverageResponse{Periods: periods}), nil
}

func (s *Server) ListStatementAlerts(ctx context.Context, _ *connect.Request[pb.ListStatementAlertsRequest]) (*connect.Response[pb.ListStatementAlertsResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	alerts, err := s.services.Statements.Alerts(ctx, userID)
	if err != nil {
		return nil, wrapErr(err)
	}

	return connect.NewResponse(&pb.ListStatementAlertsResponse{Alerts: alerts}), nil
}
