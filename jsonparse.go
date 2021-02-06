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

// CreateJSONParser is
func CreateJSONParser(str string) (m map[string]interface{}, err error) {
	jsonobj := map[string]interface{}{}
	unmarsha1Err := json.Unmarshal([]byte(str), &jsonobj)
	if unmarsha1Err != nil {
		return nil, unmarsha1Err
	}
	return jsonobj, nil
}
