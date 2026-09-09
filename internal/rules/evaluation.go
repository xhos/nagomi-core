package rules

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"nagomi-core/internal/db/sqlc"
)

// EvaluateRule evaluates a rule against transaction and account data
func EvaluateRule(rule *RuleConditions, tx *sqlc.Transaction, account *sqlc.GetAccountRow) (bool, error) {
	if rule == nil || tx == nil {
		return false, nil
	}

	switch LogicOperator(rule.Logic) {
	case LogicAND:
		// All conditions must be true
		for _, condition := range rule.Conditions {
			matches, err := evaluateCondition(&condition, tx, account)
			if err != nil {
				return false, err
			}
			if !matches {
				return false, nil
			}
		}
		return true, nil

	case LogicOR:
		// At least one condition must be true
		for _, condition := range rule.Conditions {
			matches, err := evaluateCondition(&condition, tx, account)
			if err != nil {
				return false, err
			}
			if matches {
				return true, nil
			}
		}
		return false, nil

	default:
		return false, nil
	}
}

// evaluateCondition evaluates a single condition against transaction data
func evaluateCondition(condition *Condition, tx *sqlc.Transaction, account *sqlc.GetAccountRow) (bool, error) {
	field := FieldType(condition.Field)
	operator := OperatorType(condition.Operator)

	// Get the field value from transaction data
	var fieldValue *string
	var numericValue *float64

	switch field {
	case FieldMerchant:
		fieldValue = tx.Merchant
	case FieldTxDesc:
		fieldValue = tx.TxDesc
	case FieldAccountType:
		if account != nil {
			accountType := account.Account.AccountType.String()
			fieldValue = &accountType
		}
	case FieldAccountName:
		if account != nil {
			fieldValue = &account.Account.Name
		}
	case FieldBank:
		if account != nil {
			fieldValue = &account.Account.Bank
		}
	case FieldCurrency:
		fieldValue = &tx.TxCurrency
	case FieldAmount:
		// Convert cents to dollars for rule matching
		amount := float64(tx.TxAmountCents) / 100.0
		numericValue = &amount
	case FieldTxDirection:
		val := float64(tx.TxDirection)
		numericValue = &val
	default:
		return false, nil
	}

	// Handle string fields
	if IsStringField(field) {
		return evaluateStringCondition(operator, fieldValue, condition)
	}

	// Handle numeric fields
	if IsNumericField(field) {
		return evaluateNumericCondition(operator, numericValue, condition)
	}

	return false, nil
}

func evaluateStringCondition(operator OperatorType, fieldValue *string, condition *Condition) (bool, error) {
	if fieldValue == nil {
		return false, nil
	}

	value := *fieldValue
	caseSensitive := condition.CaseSensitive != nil && *condition.CaseSensitive

	if !caseSensitive {
		value = strings.ToLower(value)
	}

	switch operator {
	case OpEquals:
		return evaluateStringEquals(value, condition, caseSensitive)
	case OpNotEquals:
		result, err := evaluateStringEquals(value, condition, caseSensitive)
		return !result, err
	case OpContains:
		return evaluateStringContains(value, condition, caseSensitive)
	case OpNotContains:
		result, err := evaluateStringContains(value, condition, caseSensitive)
		return !result, err
	case OpStartsWith:
		return evaluateStringStartsWith(value, condition, caseSensitive)
	case OpEndsWith:
		return evaluateStringEndsWith(value, condition, caseSensitive)
	case OpContainsAny:
		return evaluateStringContainsAny(value, condition, caseSensitive)
	case OpRegex:
		return evaluateRegexPattern(*fieldValue, condition, caseSensitive)
	default:
		return false, nil
	}
}

func evaluateStringEquals(value string, condition *Condition, caseSensitive bool) (bool, error) {
	compareValue, err := getStringValue(condition.Value)
	if err != nil {
		return false, err
	}

	if !caseSensitive {
		compareValue = strings.ToLower(compareValue)
	}

	return value == compareValue, nil
}

func evaluateStringContains(value string, condition *Condition, caseSensitive bool) (bool, error) {
	compareValue, err := getStringValue(condition.Value)
	if err != nil {
		return false, err
	}

	if !caseSensitive {
		compareValue = strings.ToLower(compareValue)
	}

	return strings.Contains(value, compareValue), nil
}

func evaluateStringStartsWith(value string, condition *Condition, caseSensitive bool) (bool, error) {
	compareValue, err := getStringValue(condition.Value)
	if err != nil {
		return false, err
	}

	if !caseSensitive {
		compareValue = strings.ToLower(compareValue)
	}

	return strings.HasPrefix(value, compareValue), nil
}

func evaluateStringEndsWith(value string, condition *Condition, caseSensitive bool) (bool, error) {
	compareValue, err := getStringValue(condition.Value)
	if err != nil {
		return false, err
	}

	if !caseSensitive {
		compareValue = strings.ToLower(compareValue)
	}

	return strings.HasSuffix(value, compareValue), nil
}

func evaluateStringContainsAny(value string, condition *Condition, caseSensitive bool) (bool, error) {
	for _, compareValue := range condition.Values {
		if !caseSensitive {
			compareValue = strings.ToLower(compareValue)
		}
		if strings.Contains(value, compareValue) {
			return true, nil
		}
	}
	return false, nil
}

func evaluateRegexPattern(originalValue string, condition *Condition, caseSensitive bool) (bool, error) {
	pattern, err := getStringValue(condition.Value)
	if err != nil {
		return false, err
	}

	if !caseSensitive {
		pattern = "(?i)" + pattern
	}

	regex, err := regexp.Compile(pattern)
	if err != nil {
		return false, err
	}

	return regex.MatchString(originalValue), nil
}

func evaluateNumericCondition(operator OperatorType, fieldValue *float64, condition *Condition) (bool, error) {
	if fieldValue == nil {
		return false, nil
	}

	value := *fieldValue

	switch operator {
	case OpEquals:
		return evaluateNumericEquals(value, condition)
	case OpNotEquals:
		result, err := evaluateNumericEquals(value, condition)
		return !result, err
	case OpGreaterThan:
		return evaluateNumericComparison(value, condition, func(v, c float64) bool { return v > c })
	case OpLessThan:
		return evaluateNumericComparison(value, condition, func(v, c float64) bool { return v < c })
	case OpBetween:
		return evaluateNumericBetween(value, condition)
	default:
		return false, nil
	}
}

func evaluateNumericEquals(value float64, condition *Condition) (bool, error) {
	compareValue, err := getNumericValue(condition.Value)
	if err != nil {
		return false, err
	}
	return value == compareValue, nil
}

func evaluateNumericComparison(value float64, condition *Condition, compare func(float64, float64) bool) (bool, error) {
	compareValue, err := getNumericValue(condition.Value)
	if err != nil {
		return false, err
	}
	return compare(value, compareValue), nil
}

func evaluateNumericBetween(value float64, condition *Condition) (bool, error) {
	if condition.MinValue == nil || condition.MaxValue == nil {
		return false, nil
	}

	isInRange := value >= *condition.MinValue && value <= *condition.MaxValue
	return isInRange, nil
}

// GetRuleFieldsUsed returns a list of fields that a rule uses
// This can be helpful for determining what data is needed for evaluation
func GetRuleFieldsUsed(rule *RuleConditions) []FieldType {
	if rule == nil {
		return nil
	}

	fieldsSet := make(map[FieldType]bool)
	for _, condition := range rule.Conditions {
		field := FieldType(condition.Field)
		fieldsSet[field] = true
	}

	fields := make([]FieldType, 0, len(fieldsSet))
	for field := range fieldsSet {
		fields = append(fields, field)
	}

	return fields
}

// GetRuleComplexity returns a simple complexity score for a rule
// Higher scores indicate more complex rules
func GetRuleComplexity(rule *RuleConditions) int {
	if rule == nil {
		return 0
	}

	complexity := len(rule.Conditions)

	// Add complexity for specific operators
	for _, condition := range rule.Conditions {
		operator := OperatorType(condition.Operator)
		switch operator {
		case OpRegex:
			complexity += 2 // Regex is more expensive
		case OpContainsAny:
			complexity += len(condition.Values) - 1 // Multiple comparisons
		case OpBetween:
			complexity += 1 // Two comparisons
		}
	}

	return complexity
}

// GetRuleDescription returns a human-readable description of the rule
// This can be used for UI display or logging
func GetRuleDescription(rule *RuleConditions) string {
	if rule == nil || len(rule.Conditions) == 0 {
		return "Empty rule"
	}

	descriptions := make([]string, len(rule.Conditions))
	for i, condition := range rule.Conditions {
		descriptions[i] = getConditionDescription(&condition)
	}

	logic := strings.ToLower(rule.Logic)
	if len(descriptions) == 1 {
		return descriptions[0]
	}

	return strings.Join(descriptions, " "+logic+" ")
}

func getConditionDescription(condition *Condition) string {
	field := strings.ReplaceAll(condition.Field, "_", " ")
	operator := OperatorType(condition.Operator)

	switch operator {
	case OpEquals:
		return describeEqualsOperation(field, condition, "equals")
	case OpNotEquals:
		return describeEqualsOperation(field, condition, "does not equal")
	case OpContains:
		return describeStringOperation(field, condition, "contains")
	case OpNotContains:
		return describeStringOperation(field, condition, "does not contain")
	case OpStartsWith:
		return describeStringOperation(field, condition, "starts with")
	case OpEndsWith:
		return describeStringOperation(field, condition, "ends with")
	case OpContainsAny:
		return field + " contains any of [" + strings.Join(condition.Values, ", ") + "]"
	case OpRegex:
		return describeStringOperation(field, condition, "matches pattern")
	case OpGreaterThan:
		return describeNumericOperation(field, condition, ">")
	case OpLessThan:
		return describeNumericOperation(field, condition, "<")
	case OpBetween:
		return fmt.Sprintf("%s between %s and %s", field, formatFloat(*condition.MinValue), formatFloat(*condition.MaxValue))
	default:
		return field + " " + condition.Operator + " (unknown)"
	}
}

func describeEqualsOperation(field string, condition *Condition, verb string) string {
	isStringField := IsStringField(FieldType(condition.Field))
	if isStringField {
		value, _ := getStringValue(condition.Value)
		return fmt.Sprintf("%s %s '%s'", field, verb, value)
	}

	value, _ := getNumericValue(condition.Value)
	return fmt.Sprintf("%s %s %s", field, verb, formatFloat(value))
}

func describeStringOperation(field string, condition *Condition, verb string) string {
	value, _ := getStringValue(condition.Value)
	return fmt.Sprintf("%s %s '%s'", field, verb, value)
}

func describeNumericOperation(field string, condition *Condition, symbol string) string {
	value, _ := getNumericValue(condition.Value)
	return fmt.Sprintf("%s %s %s", field, symbol, formatFloat(value))
}

func formatFloat(f float64) string {
	s := fmt.Sprintf("%.2f", f)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	return s
}

// getStringValue converts an interface{} to string
func getStringValue(value interface{}) (string, error) {
	if value == nil {
		return "", fmt.Errorf("value is nil")
	}

	switch v := value.(type) {
	case string:
		return v, nil
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	case int:
		return strconv.Itoa(v), nil
	case int64:
		return strconv.FormatInt(v, 10), nil
	default:
		return fmt.Sprintf("%v", v), nil
	}
}

// getNumericValue converts an interface{} to float64
func getNumericValue(value interface{}) (float64, error) {
	if value == nil {
		return 0, fmt.Errorf("value is nil")
	}

	switch v := value.(type) {
	case float64:
		return v, nil
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case string:
		return strconv.ParseFloat(v, 64)
	default:
		return 0, fmt.Errorf("cannot convert %T to float64", value)
	}
}
