package websearch

import (
	"context"
	"errors"
	"testing"
	"time"

	"arkloop/services/worker/internal/tools"
)

func TestParseArgsAcceptQueriesArray(t *testing.T) {
	queries, maxResults, err := parseArgs(map[string]any{
		"queries":     []any{"  q1 ", "q2"},
		"max_results": float64(3),
	})
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}
	if maxResults != 3 {
		t.Fatalf("expected maxResults=3, got %d", maxResults)
	}
	if len(queries) != 2 || queries[0] != "q1" || queries[1] != "q2" {
		t.Fatalf("unexpected queries: %#v", queries)
	}
}

func TestParseArgsRejectTooManyQueries(t *testing.T) {
	_, _, err := parseArgs(map[string]any{
		"queries":     []any{"q1", "q2", "q3", "q4", "q5", "q6"},
		"max_results": 1,
	})
	if err == nil {
		t.Fatal("expected parseArgs to fail")
	}
	if err.ErrorClass != errorArgsInvalid {
		t.Fatalf("unexpected error class: %s", err.ErrorClass)
	}
}

func TestParseArgsDefaultsMaxResults(t *testing.T) {
	queries, maxResults, err := parseArgs(map[string]any{
		"query": "q1",
	})
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}
	if maxResults != defaultMaxResults {
		t.Fatalf("expected maxResults=%d, got %d", defaultMaxResults, maxResults)
	}
	if len(queries) != 1 || queries[0] != "q1" {
		t.Fatalf("unexpected queries: %#v", queries)
	}
}

func TestExecuteMultiSearchPartialFailure(t *testing.T) {
	executor := &ToolExecutor{
		provider: stubProvider{
			resultsByQuery: map[string][]Result{
				"ok": {{Title: "A", URL: "https://a.example"}},
			},
			errorsByQuery: map[string]error{
				"bad": HttpError{StatusCode: 500},
			},
		},
		timeout: 2 * time.Second,
	}

	result := executor.Execute(
		context.Background(),
		"web_search",
		map[string]any{
			"queries":     []any{"ok", "bad"},
			"max_results": 5,
		},
		tools.ExecutionContext{},
		"call_1",
	)
	if result.Error != nil {
		t.Fatalf("expected partial success, got error: %#v", result.Error)
	}

	meta, ok := result.ResultJSON["meta"].(map[string]any)
	if !ok {
		t.Fatalf("missing meta in result: %#v", result.ResultJSON)
	}
	if meta["succeeded_queries"] != 1 {
		t.Fatalf("expected succeeded_queries=1, got %#v", meta["succeeded_queries"])
	}
	if meta["failed_queries"] != 1 {
		t.Fatalf("expected failed_queries=1, got %#v", meta["failed_queries"])
	}
}

func TestExecuteMultiSearchAllFailed(t *testing.T) {
	executor := &ToolExecutor{
		provider: stubProvider{
			errorsByQuery: map[string]error{
				"q1": errors.New("boom"),
				"q2": errors.New("boom"),
			},
		},
		timeout: 2 * time.Second,
	}

	result := executor.Execute(
		context.Background(),
		"web_search",
		map[string]any{
			"queries":     []any{"q1", "q2"},
			"max_results": 5,
		},
		tools.ExecutionContext{},
		"call_1",
	)
	if result.Error == nil {
		t.Fatal("expected error when all queries fail")
	}
	if result.Error.ErrorClass != errorSearchFailed {
		t.Fatalf("unexpected error class: %s", result.Error.ErrorClass)
	}
}

type stubProvider struct {
	resultsByQuery map[string][]Result
	errorsByQuery  map[string]error
}

func (s stubProvider) Search(ctx context.Context, query string, maxResults int) ([]Result, error) {
	_ = ctx
	_ = maxResults
	if err, ok := s.errorsByQuery[query]; ok {
		return nil, err
	}
	if results, ok := s.resultsByQuery[query]; ok {
		return results, nil
	}
	return []Result{}, nil
}
