package handlers

import (
    "fmt"
)

// resolveEchoToString normalize echo/request_id into a string representation
// It supports string, int, int64, float64 and others that can be represented
func resolveEchoToString(echoVal interface{}) (string, bool) {
    switch v := echoVal.(type) {
    case string:
        if v == "" {
            return "", false
        }
        return v, true
    case int:
        return fmt.Sprint(v), true
    case int64:
        return fmt.Sprint(v), true
    case float64:
        return fmt.Sprintf("%.0f", v), true
    default:
        return "", false
    }
}
