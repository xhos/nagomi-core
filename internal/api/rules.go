package api

import (
	"context"
	"encoding/json"
	"errors"

	pb "nagomi-core/internal/gen/nagomi/v1"
	"nagomi-core/internal/rules"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/structpb"
)

// TODO: validation can be simplified

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

	ruleID, err := parseUUID(req.Msg.GetRuleId())
	if err != nil {
		return nil, err
	}

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

	// validate that at least one action (category or merchant) is specified
	if req.Msg.CategoryId == nil && req.Msg.Merchant == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("at least one action (category_id or merchant) must be specified"))
	}

	conditionsBytes, err := req.Msg.GetConditions().MarshalJSON()
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid conditions JSON"))
	}

	validationResult := rules.ValidateRuleJSONDetailed(conditionsBytes)
	if !validationResult.Valid {
		errorMsg := "rule validation failed:"
		for _, validationErr := range validationResult.Errors {
			errorMsg += " " + validationErr.Error() + ";"
		}
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New(errorMsg))
	}

	normalizedRule, err := rules.NormalizeAndValidateRule(conditionsBytes)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("rule normalization failed: "+err.Error()))
	}

	normalizedBytes, err := json.Marshal(normalizedRule)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to serialize normalized rule"))
	}
	conditionsBytes = normalizedBytes

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

	ruleID, err := parseUUID(req.Msg.GetRuleId())
	if err != nil {
		return nil, err
	}

	var conditionsBytes []byte
	if req.Msg.Conditions != nil {
		condBytes, err := req.Msg.Conditions.MarshalJSON()
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid conditions JSON"))
		}

		validationResult := rules.ValidateRuleJSONDetailed(condBytes)
		if !validationResult.Valid {
			errorMsg := "rule validation failed:"
			for _, validationErr := range validationResult.Errors {
				errorMsg += " " + validationErr.Error() + ";"
			}
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New(errorMsg))
		}

		normalizedRule, err := rules.NormalizeAndValidateRule(condBytes)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("rule normalization failed: "+err.Error()))
		}

		normalizedBytes, err := json.Marshal(normalizedRule)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to serialize normalized rule"))
		}
		conditionsBytes = normalizedBytes
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

	ruleID, err := parseUUID(req.Msg.GetRuleId())
	if err != nil {
		return nil, err
	}

	affected, err := s.services.Rules.Delete(ctx, userID, ruleID)
	if err != nil {
		return nil, wrapErr(err)
	}

	return connect.NewResponse(&pb.DeleteRuleResponse{
		AffectedRows: affected,
	}), nil
}

func (s *Server) ValidateRule(ctx context.Context, req *connect.Request[pb.ValidateRuleRequest]) (*connect.Response[pb.ValidateRuleResponse], error) {
	conditionsBytes, err := req.Msg.GetConditions().MarshalJSON()
	if err != nil {
		return connect.NewResponse(&pb.ValidateRuleResponse{
			Valid: false,
			Errors: []*pb.ValidationError{{
				Field:   "conditions",
				Message: "Invalid JSON: " + err.Error(),
				Code:    "INVALID_JSON",
			}},
		}), nil
	}

	validationResult := rules.ValidateRuleJSONDetailed(conditionsBytes)

	response := &pb.ValidateRuleResponse{
		Valid:  validationResult.Valid,
		Errors: make([]*pb.ValidationError, len(validationResult.Errors)),
	}

	for i, validationErr := range validationResult.Errors {
		response.Errors[i] = &pb.ValidationError{
			Field:   validationErr.Field,
			Message: validationErr.Message,
			Code:    validationErr.Code,
		}
	}

	if validationResult.Valid {
		normalizedRule, err := rules.NormalizeAndValidateRule(conditionsBytes)
		if err == nil {
			normalizedBytes, err := json.Marshal(normalizedRule)
			if err == nil {
				var normalizedStruct structpb.Struct
				if err := normalizedStruct.UnmarshalJSON(normalizedBytes); err == nil {
					response.NormalizedConditions = &normalizedStruct
				}
			}
		}
	}

	return connect.NewResponse(response), nil
}
