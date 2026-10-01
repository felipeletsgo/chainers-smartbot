package main

import (
	"fmt"
	"strconv"
)

// getString extrai uma string de um map[string]interface{} com segurança
func getString(m map[string]interface{}, key string) string {
	if val, ok := m[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

// getInt extrai um int de um map[string]interface{} com segurança
func getInt(m map[string]interface{}, key string) int {
	if val, ok := m[key]; ok {
		switch v := val.(type) {
		case int:
			return v
		case int64:
			return int(v)
		case float64:
			return int(v)
		case string:
			i, err := strconv.Atoi(v)
			if err == nil {
				return i
			}
		}
	}
	return 0
}

// getFloat extrai um float64 de um map[string]interface{} com segurança
func getFloat(m map[string]interface{}, key string) float64 {
	if val, ok := m[key]; ok {
		switch v := val.(type) {
		case float64:
			return v
		case float32:
			return float64(v)
		case int:
			return float64(v)
		case int64:
			return float64(v)
		case string:
			f, err := strconv.ParseFloat(v, 64)
			if err == nil {
				return f
			}
		}
	}
	return 0
}

// getBool extrai um bool de um map[string]interface{} com segurança
func getBool(m map[string]interface{}, key string) bool {
	if val, ok := m[key]; ok {
		if b, ok := val.(bool); ok {
			return b
		}
		// Tenta string "true"/"false"
		if str, ok := val.(string); ok {
			return str == "true" || str == "True" || str == "TRUE"
		}
	}
	return false
}

// formatNumber formata um número com separadores de milhar
func formatNumber(n float64) string {
	s := fmt.Sprintf("%.0f", n)
	if len(s) <= 3 {
		return s
	}
	
	// Adiciona vírgulas
	result := ""
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result += ","
		}
		result += string(c)
	}
	return result
}
