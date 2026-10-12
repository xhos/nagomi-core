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
