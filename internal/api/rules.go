package api

import (
	"context"
	"encoding/json"

	pb "nagomi-core/internal/gen/nagomi/v1"
	"nagomi-core/internal/rules"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/structpb"
)

// normalizeConditions validates rule conditions and returns them normalized
func normalizeConditions(conditions *structpb.Struct) ([]byte, error) {
	raw, err := conditions.MarshalJSON()
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	rule, err := rules.NormalizeAndValidateRule(raw)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	return json.Marshal(rule)
}

func (s *Server) ListRules(ctx context.Context, req *connect.Request[pb.ListRulesRequest]) (*connect.Response[pb.ListRulesResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	rules, err := s.services.Rules.List(ctx, userID)
	if err != nil {
		return nil, wrapErr(err)
	}

	return connect.NewResponse(&pb.ListRulesResponse{
		Rules: rules,
	}), nil
}

func (s *Server) GetRule(ctx context.Context, req *connect.Request[pb.GetRuleRequest]) (*connect.Response[pb.GetRuleResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	ruleID := uuid.MustParse(req.Msg.GetRuleId())

	rule, err := s.services.Rules.Get(ctx, userID, ruleID)
	if err != nil {
		return nil, wrapErr(err)
	}

	return connect.NewResponse(&pb.GetRuleResponse{
		Rule: rule,
	}), nil
}

func (s *Server) CreateRule(ctx context.Context, req *connect.Request[pb.CreateRuleRequest]) (*connect.Response[pb.CreateRuleResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	conditionsBytes, err := normalizeConditions(req.Msg.GetConditions())
	if err != nil {
		return nil, err
	}

	rule, err := s.services.Rules.Create(ctx, userID, req.Msg.GetRuleName(), conditionsBytes, req.Msg.CategoryId, req.Msg.Merchant)
	if err != nil {
		return nil, wrapErr(err)
	}

	// apply to existing transactions if requested
	if req.Msg.ApplyToExisting != nil && *req.Msg.ApplyToExisting {
		count, err := s.services.Rules.ApplyToExisting(ctx, userID, nil)
		if err != nil {
			// log but don't fail the request
			s.log.Warn("failed to apply rule to existing transactions", "rule_id", rule.RuleId, "error", err)
		} else {
			s.log.Info("applied rule to existing transactions", "rule_id", rule.RuleId, "count", count)
		}
	}

	return connect.NewResponse(&pb.CreateRuleResponse{
		Rule: rule,
	}), nil
}

func (s *Server) UpdateRule(ctx context.Context, req *connect.Request[pb.UpdateRuleRequest]) (*connect.Response[pb.UpdateRuleResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	ruleID := uuid.MustParse(req.Msg.GetRuleId())

	var conditionsBytes []byte
	if req.Msg.Conditions != nil {
		if conditionsBytes, err = normalizeConditions(req.Msg.Conditions); err != nil {
			return nil, err
		}
	}

	err = s.services.Rules.Update(ctx, userID, ruleID, req.Msg.RuleName, conditionsBytes, req.Msg.CategoryId, req.Msg.Merchant)
	if err != nil {
		return nil, wrapErr(err)
	}

	// apply to existing transactions if requested
	if req.Msg.ApplyToExisting != nil && *req.Msg.ApplyToExisting {
		count, err := s.services.Rules.ApplyToExisting(ctx, userID, nil)
		if err != nil {
			s.log.Warn("failed to apply rule to existing transactions", "rule_id", ruleID, "error", err)
		} else {
			s.log.Info("applied rule to existing transactions", "rule_id", ruleID, "count", count)
		}
	}

	return connect.NewResponse(&pb.UpdateRuleResponse{}), nil
}

func (s *Server) DeleteRule(ctx context.Context, req *connect.Request[pb.DeleteRuleRequest]) (*connect.Response[pb.DeleteRuleResponse], error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}

	ruleID := uuid.MustParse(req.Msg.GetRuleId())

	affected, err := s.services.Rules.Delete(ctx, userID, ruleID)
	if err != nil {
		return nil, wrapErr(err)
	}

	return connect.NewResponse(&pb.DeleteRuleResponse{
		AffectedRows: affected,
	}), nil
}
