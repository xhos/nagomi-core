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
