package executor

import (
	"fmt"
	"math"
	"strings"

	"arkloop/services/worker/internal/pipeline"

	"github.com/google/uuid"
)

func requiredString(values map[string]any, key string) (string, error) {
	raw, ok := values[key]
	if !ok {
		return "", fmt.Errorf("missing %s", key)
	}
	text, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("%s must be a string", key)
	}
	cleaned := strings.TrimSpace(text)
	if cleaned == "" {
		return "", fmt.Errorf("%s must not be empty", key)
	}
	return cleaned, nil
}

func requiredUUID(values map[string]any, key string) (uuid.UUID, error) {
	text, err := requiredString(values, key)
	if err != nil {
		return uuid.Nil, err
	}
	id, err := uuid.Parse(text)
	if err != nil {
		return uuid.Nil, fmt.Errorf("%s is not a valid UUID", key)
	}
	return id, nil
}

func requiredInt64(values map[string]any, key string) (int64, error) {
	raw, ok := values[key]
	if !ok {
		return 0, fmt.Errorf("missing %s", key)
	}
	value, ok := numberToInt64(raw)
	if !ok {
		return 0, fmt.Errorf("%s must be a number", key)
	}
	return value, nil
}

func optionalInt(values map[string]any, key string) (int, error) {
	raw, ok := values[key]
	if !ok || raw == nil {
		return 0, nil
	}
	value, ok := numberToInt64(raw)
	if !ok {
		return 0, fmt.Errorf("%s must be a number", key)
	}
	if value > math.MaxInt || value < math.MinInt {
		return 0, fmt.Errorf("%s is out of range", key)
	}
	return int(value), nil
}

func numberToInt64(raw any) (int64, bool) {
	switch value := raw.(type) {
	case int:
		return int64(value), true
	case int8:
		return int64(value), true
	case int16:
		return int64(value), true
	case int32:
		return int64(value), true
	case int64:
		return value, true
	case uint:
		if uint64(value) > math.MaxInt64 {
			return 0, false
		}
		return int64(value), true
	case uint8:
		return int64(value), true
	case uint16:
		return int64(value), true
	case uint32:
		return int64(value), true
	case uint64:
		if value > math.MaxInt64 {
			return 0, false
		}
		return int64(value), true
	case float32:
		if math.Trunc(float64(value)) != float64(value) {
			return 0, false
		}
		return int64(value), true
	case float64:
		if math.Trunc(value) != value {
			return 0, false
		}
		return int64(value), true
	default:
		return 0, false
	}
}

func stringPtr(value string) *string {
	cleaned := strings.TrimSpace(value)
	if cleaned == "" {
		return nil
	}
	return &cleaned
}

// agentIDFromPersona 返回当前 run 的稳定 memory bucket。
func agentIDFromPersona(rc *pipeline.RunContext) string {
	return pipeline.StableAgentID(rc)
}
