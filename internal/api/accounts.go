package api

import (
	"context"

	pb "nagomi-core/internal/gen/nagomi/v1"

	"connectrpc.com/connect"
)

func (s *Server) CreateAccount(ctx context.Context, req *connect.Request[pb.CreateAccountRequest]) (*connect.Response[pb.CreateAccountResponse], error) {
	account, err := s.services.Accounts.Create(ctx, req.Msg)
	if err != nil {
		return nil, wrapErr(err)
	}

	return connect.NewResponse(&pb.CreateAccountResponse{Account: account}), nil
}

func (s *Server) GetAccount(ctx context.Context, req *connect.Request[pb.GetAccountRequest]) (*connect.Response[pb.GetAccountResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	account, err := s.services.Accounts.Get(ctx, userID, req.Msg.GetId())
	if err != nil {
		return nil, wrapErr(err)
	}

	return connect.NewResponse(&pb.GetAccountResponse{Account: account}), nil
}

func (s *Server) UpdateAccount(ctx context.Context, req *connect.Request[pb.UpdateAccountRequest]) (*connect.Response[pb.UpdateAccountResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	err = s.services.Accounts.Update(ctx, userID, req.Msg)
	if err != nil {
		return nil, wrapErr(err)
	}

	return connect.NewResponse(&pb.UpdateAccountResponse{}), nil
}

func (s *Server) DeleteAccount(ctx context.Context, req *connect.Request[pb.DeleteAccountRequest]) (*connect.Response[pb.DeleteAccountResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	affectedRows, err := s.services.Accounts.Delete(ctx, userID, req.Msg.GetId())
	if err != nil {
		return nil, wrapErr(err)
	}

	return connect.NewResponse(&pb.DeleteAccountResponse{AffectedRows: affectedRows}), nil
}

func (s *Server) ListAccounts(ctx context.Context, req *connect.Request[pb.ListAccountsRequest]) (*connect.Response[pb.ListAccountsResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	accounts, err := s.services.Accounts.List(ctx, userID)
	if err != nil {
		return nil, wrapErr(err)
	}

	return connect.NewResponse(&pb.ListAccountsResponse{Accounts: accounts}), nil
}

func (s *Server) AddAccountAlias(ctx context.Context, req *connect.Request[pb.AddAccountAliasRequest]) (*connect.Response[pb.AddAccountAliasResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	if err := s.services.Accounts.AddAlias(ctx, userID, req.Msg.GetAccountId(), req.Msg.GetAlias()); err != nil {
		return nil, wrapErr(err)
	}

	return connect.NewResponse(&pb.AddAccountAliasResponse{}), nil
}

func (s *Server) RemoveAccountAlias(ctx context.Context, req *connect.Request[pb.RemoveAccountAliasRequest]) (*connect.Response[pb.RemoveAccountAliasResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	if err := s.services.Accounts.RemoveAlias(ctx, userID, req.Msg.GetAccountId(), req.Msg.GetAlias()); err != nil {
		return nil, wrapErr(err)
	}

	return connect.NewResponse(&pb.RemoveAccountAliasResponse{}), nil
}

func (s *Server) SetAccountAliases(ctx context.Context, req *connect.Request[pb.SetAccountAliasesRequest]) (*connect.Response[pb.SetAccountAliasesResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	if err := s.services.Accounts.SetAliases(ctx, userID, req.Msg.GetAccountId(), req.Msg.GetAliases()); err != nil {
		return nil, wrapErr(err)
	}

	return connect.NewResponse(&pb.SetAccountAliasesResponse{}), nil
}

func (s *Server) FindAccountByAlias(ctx context.Context, req *connect.Request[pb.FindAccountByAliasRequest]) (*connect.Response[pb.FindAccountByAliasResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	account, err := s.services.Accounts.FindByAlias(ctx, userID, req.Msg.GetAlias())
	if err != nil {
		return nil, wrapErr(err)
	}

	return connect.NewResponse(&pb.FindAccountByAliasResponse{Account: account}), nil
}

func (s *Server) MergeAccounts(ctx context.Context, req *connect.Request[pb.MergeAccountsRequest]) (*connect.Response[pb.MergeAccountsResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	account, moved, err := s.services.Accounts.MergeAccounts(ctx, userID, req.Msg.GetPrimaryAccountId(), req.Msg.GetSecondaryAccountId())
	if err != nil {
		return nil, wrapErr(err)
	}

	return connect.NewResponse(&pb.MergeAccountsResponse{Account: account, TransactionsMoved: moved}), nil
}
