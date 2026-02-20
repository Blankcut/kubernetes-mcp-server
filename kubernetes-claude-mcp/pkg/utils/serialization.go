package utils

import (
	"encoding/json"
	"fmt"
)

// ToJSON converts a struct to a JSON string with pretty formatting
func ToJSON(v interface{}) (string, error) {
	jsonBytes, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal to JSON: %w", err)
	}
	return string(jsonBytes), nil
}

// FromJSON converts a JSON string to a struct
func FromJSON(data string, v interface{}) error {
	err := json.Unmarshal([]byte(data), v)
	if err != nil {
		return fmt.Errorf("failed to unmarshal from JSON: %w", err)
	}
	return nil
}

// ToMap converts a struct to a map[string]interface{}
func ToMap(v interface{}) (map[string]interface{}, error) {
	jsonBytes, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal to JSON: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal to map: %w", err)
	}

	return result, nil
}
