package api

import (
	"context"

	pb "nagomi-core/internal/gen/nagomi/v1"

	"connectrpc.com/connect"
)

func (s *Server) ListTransactions(ctx context.Context, req *connect.Request[pb.ListTransactionsRequest]) (*connect.Response[pb.ListTransactionsResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	transactions, nextCursor, err := s.services.Transactions.List(ctx, userID, req.Msg)
	if err != nil {
		return nil, wrapErr(err)
	}

	total, err := s.services.Transactions.Count(ctx, userID, req.Msg)
	if err != nil {
		return nil, wrapErr(err)
	}

	return connect.NewResponse(&pb.ListTransactionsResponse{
		Transactions: transactions,
		TotalCount:   total,
		NextCursor:   nextCursor,
	}), nil
}

func (s *Server) GetTransaction(ctx context.Context, req *connect.Request[pb.GetTransactionRequest]) (*connect.Response[pb.GetTransactionResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	transaction, err := s.services.Transactions.Get(ctx, userID, req.Msg.GetId())
	if err != nil {
		return nil, wrapErr(err)
	}

	return connect.NewResponse(&pb.GetTransactionResponse{
		Transaction: transaction,
	}), nil
}

func (s *Server) CreateTransaction(ctx context.Context, req *connect.Request[pb.CreateTransactionRequest]) (*connect.Response[pb.CreateTransactionResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	transactions, err := s.services.Transactions.Create(ctx, userID, req.Msg)
	if err != nil {
		return nil, wrapErr(err)
	}

	return connect.NewResponse(&pb.CreateTransactionResponse{
		Transactions: transactions,
		CreatedCount: int32(len(transactions)),
	}), nil
}

func (s *Server) UpdateTransaction(ctx context.Context, req *connect.Request[pb.UpdateTransactionRequest]) (*connect.Response[pb.UpdateTransactionResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	err = s.services.Transactions.Update(ctx, userID, req.Msg)
	if err != nil {
		return nil, wrapErr(err)
	}

	return connect.NewResponse(&pb.UpdateTransactionResponse{}), nil
}

func (s *Server) DeleteTransaction(ctx context.Context, req *connect.Request[pb.DeleteTransactionRequest]) (*connect.Response[pb.DeleteTransactionResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	err = s.services.Transactions.Delete(ctx, userID, req.Msg.Ids)
	if err != nil {
		return nil, wrapErr(err)
	}

	return connect.NewResponse(&pb.DeleteTransactionResponse{
		AffectedRows: int64(len(req.Msg.Ids)),
	}), nil
}

func (s *Server) CategorizeTransactions(ctx context.Context, req *connect.Request[pb.CategorizeTransactionsRequest]) (*connect.Response[pb.CategorizeTransactionsResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	err = s.services.Transactions.Categorize(ctx, userID, req.Msg.TransactionIds, req.Msg.GetCategoryId())
	if err != nil {
		return nil, wrapErr(err)
	}

	return connect.NewResponse(&pb.CategorizeTransactionsResponse{
		AffectedRows: int64(len(req.Msg.TransactionIds)),
	}), nil
}

func (s *Server) SplitTransaction(ctx context.Context, req *connect.Request[pb.SplitTransactionRequest]) (*connect.Response[pb.SplitTransactionResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	splits, err := s.services.Transactions.SplitTransaction(ctx, userID, req.Msg)
	if err != nil {
		return nil, wrapErr(err)
	}

	return connect.NewResponse(&pb.SplitTransactionResponse{
		CreatedSplits: splits,
	}), nil
}

func (s *Server) ForgiveTransaction(ctx context.Context, req *connect.Request[pb.ForgiveTransactionRequest]) (*connect.Response[pb.ForgiveTransactionResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	err = s.services.Transactions.ForgiveTransaction(ctx, userID, req.Msg.GetTransactionId(), req.Msg.GetForgiven())
	if err != nil {
		return nil, wrapErr(err)
	}

	return connect.NewResponse(&pb.ForgiveTransactionResponse{}), nil
}

func (s *Server) GetFriendBalances(ctx context.Context, req *connect.Request[pb.GetFriendBalancesRequest]) (*connect.Response[pb.GetFriendBalancesResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	balances, err := s.services.Transactions.GetFriendBalances(ctx, userID)
	if err != nil {
		return nil, wrapErr(err)
	}

	return connect.NewResponse(&pb.GetFriendBalancesResponse{
		Balances: balances,
	}), nil
}
