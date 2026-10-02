package rules

import (
	_ "embed"
	"encoding/json"
	"strings"
)

//go:embed rule.schema.json
var ruleSchemaJSON []byte

// NormalizeAndValidateRule normalizes and validates a rule, returning the normalized version
func NormalizeAndValidateRule(jsonData []byte) (*RuleConditions, error) {
	rule, err := ParseRuleConditions(jsonData)
	if err != nil {
		return nil, err
	}

	// Additional normalization
	rule.Logic = strings.ToUpper(rule.Logic)

	for i := range rule.Conditions {
		condition := &rule.Conditions[i]

		// Set default case sensitivity for string fields
		if IsStringField(FieldType(condition.Field)) && condition.CaseSensitive == nil {
			defaultCase := false
			condition.CaseSensitive = &defaultCase
		}

		// Normalize string values if not case sensitive
		if IsStringField(FieldType(condition.Field)) && condition.CaseSensitive != nil && !*condition.CaseSensitive {
			if condition.Value != nil {
				if strValue, err := getStringValue(condition.Value); err == nil {
					condition.Value = strings.ToLower(strValue)
				}
			}
			for j := range condition.Values {
				condition.Values[j] = strings.ToLower(condition.Values[j])
			}
		}
	}

	return rule, nil
}

// GetValidationSchema returns a JSON schema description for frontend validation
func GetValidationSchema() map[string]interface{} {
	var schema map[string]interface{}
	if err := json.Unmarshal(ruleSchemaJSON, &schema); err != nil {
		return map[string]interface{}{}
	}

	return schema
}
