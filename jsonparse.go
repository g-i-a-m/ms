package main

import (
	"encoding/json"
	"fmt"
)

// GetValue is
func GetValue(m map[string]interface{}, key string) string {
	if _, ok := m[key]; ok {
		return fmt.Sprintf("%v", m[key])
	}
	return ""
}

// GetValue is
func GetIntValue(m map[string]interface{}, key string) float64 {
	if v, ok := m[key]; ok {
		return v.(float64)
	}
	return 0
}

// CreateJSONParser is
func CreateJSONParser(str []byte) (m map[string]interface{}, err error) {
	jsonobj := map[string]interface{}{}
	unmarsha1Err := json.Unmarshal(str, &jsonobj)
	if unmarsha1Err != nil {
		return nil, unmarsha1Err
	}
	return jsonobj, nil
}
