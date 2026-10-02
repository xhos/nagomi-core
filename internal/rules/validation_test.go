package rules

import (
	"encoding/json"
	"testing"
)

func TestValidateRuleJSON_ValidRules(t *testing.T) {
	tests := []struct {
		name string
		rule string
	}{
		{
			name: "Coffee shop rule",
			rule: `{
				"logic": "AND",
				"conditions": [
					{
						"field": "merchant",
						"operator": "contains_any",
						"values": ["starbucks", "dunkin", "peet's coffee"],
						"case_sensitive": false
					},
					{
						"field": "amount",
						"operator": "between",
						"min_value": 2.00,
						"max_value": 25.00
					}
				]
			}`,
		},
		{
			name: "Rent payment rule",
			rule: `{
				"logic": "AND",
				"conditions": [
					{
						"field": "amount",
						"operator": "equals",
						"value": 1200.00
					},
					{
						"field": "merchant",
						"operator": "contains",
						"value": "property management",
						"case_sensitive": false
					}
				]
			}`,
		},
		{
			name: "OR logic example",
			rule: `{
				"logic": "OR",
				"conditions": [
					{
						"field": "bank",
						"operator": "equals",
						"value": "Chase"
					},
					{
						"field": "bank",
						"operator": "equals",
						"value": "Wells Fargo"
					}
				]
			}`,
		},
		{
			name: "Regex example",
			rule: `{
				"logic": "AND",
				"conditions": [
					{
						"field": "merchant",
						"operator": "regex",
						"value": "^AMZN.*",
						"case_sensitive": false
					}
				]
			}`,
		},
		{
			name: "Amount greater than",
			rule: `{
				"logic": "AND",
				"conditions": [
					{
						"field": "amount",
						"operator": "greater_than",
						"value": 50.00
					}
				]
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ParseRuleConditions([]byte(tt.rule)); err != nil {
				t.Errorf("Expected rule to be valid, got: %v", err)
			}
		})
	}
}

func TestValidateRuleJSON_InvalidRules(t *testing.T) {
	tests := []struct {
		name string
		rule string
	}{
		{
			name: "Missing logic",
			rule: `{
				"conditions": [
					{
						"field": "amount",
						"operator": "equals",
						"value": 100
					}
				]
			}`,
		},
		{
			name: "Invalid logic operator",
			rule: `{
				"logic": "XOR",
				"conditions": [
					{
						"field": "amount",
						"operator": "equals",
						"value": 100
					}
				]
			}`,
		},
		{
			name: "No conditions",
			rule: `{
				"logic": "AND",
				"conditions": []
			}`,
		},
		{
			name: "Invalid field",
			rule: `{
				"logic": "AND",
				"conditions": [
					{
						"field": "invalid_field",
						"operator": "equals",
						"value": "test"
					}
				]
			}`,
		},
		{
			name: "String operator on numeric field",
			rule: `{
				"logic": "AND",
				"conditions": [
					{
						"field": "amount",
						"operator": "contains",
						"value": "test"
					}
				]
			}`,
		},
		{
			name: "Numeric operator on string field",
			rule: `{
				"logic": "AND",
				"conditions": [
					{
						"field": "merchant",
						"operator": "greater_than",
						"value": "100"
					}
				]
			}`,
		},
		{
			name: "Missing value for regular operator",
			rule: `{
				"logic": "AND",
				"conditions": [
					{
						"field": "merchant",
						"operator": "equals"
					}
				]
			}`,
		},
		{
			name: "Missing values for contains_any",
			rule: `{
				"logic": "AND",
				"conditions": [
					{
						"field": "merchant",
						"operator": "contains_any"
					}
				]
			}`,
		},
		{
			name: "Missing min/max for between",
			rule: `{
				"logic": "AND",
				"conditions": [
					{
						"field": "amount",
						"operator": "between",
						"value": 100
					}
				]
			}`,
		},
		{
			name: "Invalid range for between",
			rule: `{
				"logic": "AND",
				"conditions": [
					{
						"field": "amount",
						"operator": "between",
						"min_value": 100,
						"max_value": 50
					}
				]
			}`,
		},
		{
			name: "Case sensitive on numeric field",
			rule: `{
				"logic": "AND",
				"conditions": [
					{
						"field": "amount",
						"operator": "equals",
						"value": 100,
						"case_sensitive": true
					}
				]
			}`,
		},
		{
			name: "Currency property not supported",
			rule: `{
				"logic": "AND",
				"conditions": [
					{
						"field": "merchant",
						"operator": "equals",
						"value": "test",
						"currency": "USD"
					}
				]
			}`,
		},
		{
			name: "Invalid regex pattern",
			rule: `{
				"logic": "AND",
				"conditions": [
					{
						"field": "merchant",
						"operator": "regex",
						"value": "[invalid"
					}
				]
			}`,
		},
		{
			name: "Conflicting value fields for contains_any",
			rule: `{
				"logic": "AND",
				"conditions": [
					{
						"field": "merchant",
						"operator": "contains_any",
						"value": "test",
						"values": ["test1", "test2"]
					}
				]
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ParseRuleConditions([]byte(tt.rule)); err == nil {
				t.Errorf("Expected rule to be invalid")
			}
		})
	}
}

func TestNormalizeAndValidateRule(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name: "Normalize case insensitive string",
			input: `{
				"logic": "and",
				"conditions": [
					{
						"field": "merchant",
						"operator": "equals",
						"value": "STARBUCKS"
					}
				]
			}`,
			expected: `{
				"logic": "AND",
				"conditions": [
					{
						"field": "merchant",
						"operator": "equals",
						"value": "starbucks",
						"case_sensitive": false
					}
				]
			}`,
		},
		{
			name: "Preserve case sensitive string",
			input: `{
				"logic": "AND",
				"conditions": [
					{
						"field": "merchant",
						"operator": "equals",
						"value": "STARBUCKS",
						"case_sensitive": true
					}
				]
			}`,
			expected: `{
				"logic": "AND",
				"conditions": [
					{
						"field": "merchant",
						"operator": "equals",
						"value": "STARBUCKS",
						"case_sensitive": true
					}
				]
			}`,
		},
		{
			name: "Normalize values array",
			input: `{
				"logic": "AND",
				"conditions": [
					{
						"field": "merchant",
						"operator": "contains_any",
						"values": ["STARBUCKS", "DUNKIN"]
					}
				]
			}`,
			expected: `{
				"logic": "AND",
				"conditions": [
					{
						"field": "merchant",
						"operator": "contains_any",
						"values": ["starbucks", "dunkin"],
						"case_sensitive": false
					}
				]
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			normalized, err := NormalizeAndValidateRule([]byte(tt.input))
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			// Parse expected result for comparison
			var expected RuleConditions
			if err := json.Unmarshal([]byte(tt.expected), &expected); err != nil {
				t.Errorf("Failed to parse expected result: %v", err)
				return
			}

			// Compare logic
			if normalized.Logic != expected.Logic {
				t.Errorf("Expected logic %s, got %s", expected.Logic, normalized.Logic)
			}

			// Compare conditions
			if len(normalized.Conditions) != len(expected.Conditions) {
				t.Errorf("Expected %d conditions, got %d", len(expected.Conditions), len(normalized.Conditions))
				return
			}

			for i, condition := range normalized.Conditions {
				expectedCondition := expected.Conditions[i]

				if condition.Field != expectedCondition.Field {
					t.Errorf("Condition %d: expected field %s, got %s", i, expectedCondition.Field, condition.Field)
				}

				if condition.Operator != expectedCondition.Operator {
					t.Errorf("Condition %d: expected operator %s, got %s", i, expectedCondition.Operator, condition.Operator)
				}

				// Compare values based on what's expected
				if expectedCondition.Value != nil {
					if condition.Value == nil || condition.Value != expectedCondition.Value {
						t.Errorf("Condition %d: expected value %v, got %v", i, expectedCondition.Value, condition.Value)
					}
				}

				if len(expectedCondition.Values) > 0 {
					if len(condition.Values) != len(expectedCondition.Values) {
						t.Errorf("Condition %d: expected %d values, got %d", i, len(expectedCondition.Values), len(condition.Values))
					} else {
						for j, val := range condition.Values {
							if val != expectedCondition.Values[j] {
								t.Errorf("Condition %d, value %d: expected %s, got %s", i, j, expectedCondition.Values[j], val)
							}
						}
					}
				}

				if expectedCondition.CaseSensitive != nil {
					if condition.CaseSensitive == nil || *condition.CaseSensitive != *expectedCondition.CaseSensitive {
						t.Errorf("Condition %d: expected case_sensitive %v, got %v", i, expectedCondition.CaseSensitive, condition.CaseSensitive)
					}
				}
			}
		})
	}
}

