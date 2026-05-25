package kbapi

import (
	"context"
	"strings"

	"arkloop/services/shared/questionstore/examstore"
)

type ExamScopesLister interface {
	ListExamScopes(ctx context.Context, token string) ([]map[string]any, error)
}

type examstoreScopesLister struct {
	client *examstore.Client
}

func NewExamScopesLister(baseURL string) ExamScopesLister {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return nil
	}
	return &examstoreScopesLister{client: examstore.NewClient(baseURL)}
}

func (l *examstoreScopesLister) ListExamScopes(ctx context.Context, token string) ([]map[string]any, error) {
	resp, err := l.client.ListExamScopes(ctx, token)
	if err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0, len(resp.Items))
	for _, item := range resp.Items {
		out := map[string]any{
			"id":           item.ID,
			"type":         item.Type,
			"code":         item.Code,
			"display_name": item.DisplayName,
		}
		if item.ParentID == nil {
			out["parent_id"] = nil
		} else {
			out["parent_id"] = *item.ParentID
		}
		items = append(items, out)
	}
	return items, nil
}
