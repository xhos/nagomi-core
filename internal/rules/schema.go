package rules

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

type RuleConditions struct {
	Logic      string      `json:"logic"`
	Conditions []Condition `json:"conditions"`
}

type Condition struct {
	Field         string      `json:"field"`
	Operator      string      `json:"operator"`
	Value         interface{} `json:"value,omitempty"`
	Values        []string    `json:"values,omitempty"`
	MinValue      *float64    `json:"min_value,omitempty"`
	MaxValue      *float64    `json:"max_value,omitempty"`
	Currency      *string     `json:"currency,omitempty"`
	CaseSensitive *bool       `json:"case_sensitive,omitempty"`
}

type LogicOperator string

const (
	LogicAND LogicOperator = "AND"
	LogicOR  LogicOperator = "OR"
)

type FieldType string

const (
	FieldMerchant    FieldType = "merchant"
	FieldTxDesc      FieldType = "tx_desc"
	FieldTxDirection FieldType = "tx_direction"
	FieldAccountType FieldType = "account_type"
	FieldAccountName FieldType = "account_name"
	FieldBank        FieldType = "bank"
	FieldCurrency    FieldType = "currency"
	FieldAmount      FieldType = "amount"
)

type OperatorType string

const (
	OpEquals      OperatorType = "equals"
	OpContains    OperatorType = "contains"
	OpStartsWith  OperatorType = "starts_with"
	OpEndsWith    OperatorType = "ends_with"
	OpContainsAny OperatorType = "contains_any"
	OpNotEquals   OperatorType = "not_equals"
	OpNotContains OperatorType = "not_contains"
	OpRegex       OperatorType = "regex"
	OpGreaterThan OperatorType = "greater_than"
	OpLessThan    OperatorType = "less_than"
	OpBetween     OperatorType = "between"
)

func IsStringField(field FieldType) bool {
	return field == FieldMerchant ||
		field == FieldTxDesc ||
		field == FieldAccountType ||
		field == FieldAccountName ||
		field == FieldBank ||
		field == FieldCurrency
}

func IsNumericField(field FieldType) bool {
	return field == FieldAmount ||
		field == FieldTxDirection
}

func IsStringOperator(op OperatorType) bool {
	return op == OpEquals ||
		op == OpContains ||
		op == OpStartsWith ||
		op == OpEndsWith ||
		op == OpContainsAny ||
		op == OpNotEquals ||
		op == OpNotContains ||
		op == OpRegex
}

func IsNumericOperator(op OperatorType) bool {
	return op == OpEquals ||
		op == OpNotEquals ||
		op == OpGreaterThan ||
		op == OpLessThan ||
		op == OpBetween
}

func RequiresValues(op OperatorType) bool {
	return op == OpContainsAny
}

func RequiresMinMax(op OperatorType) bool {
	return op == OpBetween
}

func GetStringFields() []string {
	return []string{
		string(FieldMerchant),
		string(FieldTxDesc),
		string(FieldAccountType),
		string(FieldAccountName),
		string(FieldBank),
		string(FieldCurrency),
	}
}

func GetNumericFields() []string {
	return []string{
		string(FieldAmount),
		string(FieldTxDirection),
	}
}

func GetStringOperators() []string {
	return []string{
		string(OpEquals),
		string(OpContains),
		string(OpStartsWith),
		string(OpEndsWith),
		string(OpContainsAny),
		string(OpNotEquals),
		string(OpNotContains),
		string(OpRegex),
	}
}

func GetNumericOperators() []string {
	return []string{
		string(OpEquals),
		string(OpNotEquals),
		string(OpGreaterThan),
		string(OpLessThan),
		string(OpBetween),
	}
}

func ParseRuleConditions(jsonData []byte) (*RuleConditions, error) {
	var rule RuleConditions
	if err := json.Unmarshal(jsonData, &rule); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	if err := ValidateRuleConditions(&rule); err != nil {
		return nil, err
	}

	return &rule, nil
}

func ValidateRuleConditions(rule *RuleConditions) error {
	if rule == nil {
		return fmt.Errorf("rule cannot be nil")
	}

	logic := LogicOperator(strings.ToUpper(rule.Logic))
	if logic != LogicAND && logic != LogicOR {
		return fmt.Errorf("logic must be 'AND' or 'OR', got: %s", rule.Logic)
	}
	rule.Logic = string(logic)

	if len(rule.Conditions) == 0 {
		return fmt.Errorf("at least one condition is required")
	}

	for i, condition := range rule.Conditions {
		if err := ValidateCondition(&condition); err != nil {
			return fmt.Errorf("condition %d: %w", i+1, err)
		}
		rule.Conditions[i] = condition
	}

	return nil
}

func ValidateCondition(condition *Condition) error {
	if condition == nil {
		return fmt.Errorf("condition cannot be nil")
	}

	if err := validateConditionBasics(condition); err != nil {
		return err
	}

	field := FieldType(condition.Field)
	operator := OperatorType(condition.Operator)

	if err := validateConditionOperatorMatch(field, operator, condition); err != nil {
		return err
	}

	if err := validateConditionValues(operator, condition); err != nil {
		return err
	}

	if err := validateConditionFieldRules(field, condition); err != nil {
		return err
	}

	setConditionDefaults(field, condition)
	return nil
}

func validateConditionBasics(condition *Condition) error {
	field := FieldType(condition.Field)
	isValidField := IsStringField(field) || IsNumericField(field)
	if !isValidField {
		return fmt.Errorf("invalid field: %s", condition.Field)
	}
	return nil
}

func validateConditionOperatorMatch(field FieldType, operator OperatorType, condition *Condition) error {
	isStringFieldWithBadOperator := IsStringField(field) && !IsStringOperator(operator)
	isNumericFieldWithBadOperator := IsNumericField(field) && !IsNumericOperator(operator)

	if isStringFieldWithBadOperator {
		return fmt.Errorf("operator '%s' is not valid for string field '%s'", condition.Operator, condition.Field)
	}

	if isNumericFieldWithBadOperator {
		return fmt.Errorf("operator '%s' is not valid for numeric field '%s'", condition.Operator, condition.Field)
	}

	return nil
}

func validateConditionValues(operator OperatorType, condition *Condition) error {
	needsValuesArray := RequiresValues(operator)
	needsMinMax := RequiresMinMax(operator)

	if needsValuesArray {
		return validateValuesArrayCondition(operator, condition)
	}

	if needsMinMax {
		return validateMinMaxCondition(operator, condition)
	}

	return validateRegularValueCondition(operator, condition)
}

func validateValuesArrayCondition(_ OperatorType, condition *Condition) error {
	if len(condition.Values) == 0 {
		return fmt.Errorf("operator '%s' requires 'values' array", condition.Operator)
	}

	if condition.Value != nil {
		return fmt.Errorf("operator '%s' should use 'values' not 'value'", condition.Operator)
	}

	return nil
}

func validateMinMaxCondition(_ OperatorType, condition *Condition) error {
	if condition.MinValue == nil || condition.MaxValue == nil {
		return fmt.Errorf("operator '%s' requires both 'min_value' and 'max_value'", condition.Operator)
	}

	hasValidRange := *condition.MinValue < *condition.MaxValue
	if !hasValidRange {
		return fmt.Errorf("min_value must be less than max_value")
	}

	if condition.Value != nil {
		return fmt.Errorf("operator '%s' should use 'min_value'/'max_value' not 'value'", condition.Operator)
	}

	return nil
}

func validateRegularValueCondition(operator OperatorType, condition *Condition) error {
	if condition.Value == nil {
		return fmt.Errorf("operator '%s' requires 'value'", condition.Operator)
	}

	if len(condition.Values) > 0 {
		return fmt.Errorf("operator '%s' should use 'value' not 'values'", condition.Operator)
	}

	if pattern, ok := condition.Value.(string); ok && operator == OpRegex {
		if _, err := regexp.Compile(pattern); err != nil {
			return fmt.Errorf("invalid regex pattern: %w", err)
		}
	}

	return nil
}

func validateConditionFieldRules(field FieldType, condition *Condition) error {
	if condition.CaseSensitive != nil && !IsStringField(field) {
		return fmt.Errorf("case_sensitive only applies to string fields")
	}

	if condition.Currency != nil {
		return fmt.Errorf("currency property is not supported, use currency field instead")
	}

	if field == FieldAmount {
		return validateAmountFieldRules(condition)
	}

	if field == FieldTxDirection {
		return validateTxDirectionFieldRules(condition)
	}

	return nil
}

func validateAmountFieldRules(condition *Condition) error {
	if condition.MinValue != nil && *condition.MinValue < 0 {
		return fmt.Errorf("min_value cannot be negative")
	}

	if condition.MaxValue != nil && *condition.MaxValue < 0 {
		return fmt.Errorf("max_value cannot be negative")
	}

	return nil
}

func validateTxDirectionFieldRules(condition *Condition) error {
	if condition.Value != nil {
		if numValue, err := getNumericValue(condition.Value); err == nil {
			isValidDirection := numValue >= 0 && numValue <= 2
			if !isValidDirection {
				return fmt.Errorf("tx_direction must be between 0 and 2")
			}
		}
	}

	if condition.MinValue != nil {
		isValidMin := *condition.MinValue >= 0 && *condition.MinValue <= 2
		if !isValidMin {
			return fmt.Errorf("tx_direction must be between 0 and 2")
		}
	}

	if condition.MaxValue != nil {
		isValidMax := *condition.MaxValue >= 0 && *condition.MaxValue <= 2
		if !isValidMax {
			return fmt.Errorf("tx_direction must be between 0 and 2")
		}
	}

	return nil
}

func setConditionDefaults(field FieldType, condition *Condition) {
	if IsStringField(field) && condition.CaseSensitive == nil {
		defaultCase := false
		condition.CaseSensitive = &defaultCase
	}
}
