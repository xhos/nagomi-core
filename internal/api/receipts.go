package api

import (
	"context"
	"errors"

	pb "null-core/internal/gen/null/v1"
	"null-core/internal/service"

	"connectrpc.com/connect"
)

func (s *Server) UploadReceipt(ctx context.Context, req *connect.Request[pb.UploadReceiptRequest]) (*connect.Response[pb.UploadReceiptResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	receipt, err := s.services.Receipts.Upload(ctx, userID, req.Msg.GetImageData(), req.Msg.GetContentType())
	if err != nil {
		var dupErr *service.DuplicateReceiptError
		if errors.As(err, &dupErr) {
			connectErr := connect.NewError(connect.CodeAlreadyExists, errors.New("receipt has already been uploaded"))
			if detail, detailErr := connect.NewErrorDetail(&pb.Receipt{Id: dupErr.ExistingID}); detailErr == nil {
				connectErr.AddDetail(detail)
			}
			return nil, connectErr
		}
		return nil, wrapErr(err)
	}

	return connect.NewResponse(&pb.UploadReceiptResponse{
		Receipt: receipt,
	}), nil
}

func (s *Server) CreateReceipt(ctx context.Context, req *connect.Request[pb.CreateReceiptRequest]) (*connect.Response[pb.CreateReceiptResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	receipt, err := s.services.Receipts.Create(ctx, userID, req.Msg)
	if err != nil {
		return nil, wrapErr(err)
	}
	return connect.NewResponse(&pb.CreateReceiptResponse{Receipt: receipt}), nil
}

func (s *Server) ListReceipts(ctx context.Context, req *connect.Request[pb.ListReceiptsRequest]) (*connect.Response[pb.ListReceiptsResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	receipts, totalCount, err := s.services.Receipts.List(ctx, userID, req.Msg)
	if err != nil {
		return nil, wrapErr(err)
	}

	return connect.NewResponse(&pb.ListReceiptsResponse{
		Receipts:   receipts,
		TotalCount: totalCount,
	}), nil
}

func (s *Server) GetReceipt(ctx context.Context, req *connect.Request[pb.GetReceiptRequest]) (*connect.Response[pb.GetReceiptResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	receipt, linkCandidates, imageData, err := s.services.Receipts.Get(ctx, userID, req.Msg.GetId())
	if err != nil {
		return nil, wrapErr(err)
	}

	return connect.NewResponse(&pb.GetReceiptResponse{
		Receipt:        receipt,
		LinkCandidates: linkCandidates,
		ImageData:      imageData,
	}), nil
}

func (s *Server) UpdateReceipt(ctx context.Context, req *connect.Request[pb.UpdateReceiptRequest]) (*connect.Response[pb.UpdateReceiptResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	receipt, err := s.services.Receipts.Update(ctx, userID, req.Msg.GetId(), req.Msg)
	if err != nil {
		return nil, wrapErr(err)
	}

	return connect.NewResponse(&pb.UpdateReceiptResponse{
		Receipt: receipt,
	}), nil
}

func (s *Server) DeleteReceipt(ctx context.Context, req *connect.Request[pb.DeleteReceiptRequest]) (*connect.Response[pb.DeleteReceiptResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	err = s.services.Receipts.Delete(ctx, userID, req.Msg.GetId())
	if err != nil {
		return nil, wrapErr(err)
	}

	return connect.NewResponse(&pb.DeleteReceiptResponse{}), nil
}

func (s *Server) RetryParseReceipt(ctx context.Context, req *connect.Request[pb.RetryParseReceiptRequest]) (*connect.Response[pb.RetryParseReceiptResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	receipt, err := s.services.Receipts.RetryParsing(ctx, userID, req.Msg.GetId())
	if err != nil {
		return nil, wrapErr(err)
	}

	return connect.NewResponse(&pb.RetryParseReceiptResponse{Receipt: receipt}), nil
}
