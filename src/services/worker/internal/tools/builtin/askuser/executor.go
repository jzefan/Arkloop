package askuser

import (
	"context"
	"fmt"
	"time"

	"arkloop/services/worker/internal/tools"
)

const (
	errorArgsInvalid = "tool.args_invalid"
)

type ToolExecutor struct{}

func (ToolExecutor) Execute(
	_ context.Context,
	_ string,
	args map[string]any,
	_ tools.ExecutionContext,
	_ string,
) tools.ExecutionResult {
	started := time.Now()

	questions, err := validateQuestions(args)
	if err != nil {
		return tools.ExecutionResult{
			Error: &tools.ExecutionError{
				ErrorClass: errorArgsInvalid,
				Message:    err.Error(),
			},
			DurationMs: durationMs(started),
		}
	}

	return tools.ExecutionResult{
		ResultJSON: map[string]any{
			"status":    "pending_user_input",
			"questions": questions,
		},
		DurationMs: durationMs(started),
	}
}

func validateQuestions(args map[string]any) ([]any, error) {
	raw, ok := args["questions"]
	if !ok {
		return nil, fmt.Errorf("missing required field: questions")
	}
	questions, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("questions must be an array")
	}
	if len(questions) == 0 {
		return nil, fmt.Errorf("questions must not be empty")
	}
	if len(questions) > 3 {
		return nil, fmt.Errorf("questions must contain at most 3 items")
	}

	seenIDs := map[string]struct{}{}
	for i, q := range questions {
		qMap, ok := q.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("questions[%d] must be an object", i)
		}

		id, _ := qMap["id"].(string)
		if id == "" {
			return nil, fmt.Errorf("questions[%d].id must be a non-empty string", i)
		}
		if _, dup := seenIDs[id]; dup {
			return nil, fmt.Errorf("duplicate question id: %s", id)
		}
		seenIDs[id] = struct{}{}

		question, _ := qMap["question"].(string)
		if question == "" {
			return nil, fmt.Errorf("questions[%d].question must be a non-empty string", i)
		}

		options, ok := qMap["options"].([]any)
		if !ok || len(options) < 2 {
			return nil, fmt.Errorf("questions[%d].options must have at least 2 items", i)
		}
		if len(options) > 6 {
			return nil, fmt.Errorf("questions[%d].options must have at most 6 items", i)
		}

		for j, opt := range options {
			optMap, ok := opt.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("questions[%d].options[%d] must be an object", i, j)
			}
			value, _ := optMap["value"].(string)
			if value == "" {
				return nil, fmt.Errorf("questions[%d].options[%d].value must be a non-empty string", i, j)
			}
			label, _ := optMap["label"].(string)
			if label == "" {
				return nil, fmt.Errorf("questions[%d].options[%d].label must be a non-empty string", i, j)
			}
		}
	}

	return questions, nil
}

func durationMs(started time.Time) int {
	elapsed := time.Since(started)
	millis := int(elapsed / time.Millisecond)
	if millis < 0 {
		return 0
	}
	return millis
}
