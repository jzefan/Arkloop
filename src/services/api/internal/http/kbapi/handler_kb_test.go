package kbapi

import (
	"context"
	"encoding/json"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"arkloop/services/api/internal/data"

	"github.com/google/uuid"
)

type fakeKBStore struct {
	items map[uuid.UUID]*data.KnowledgeBase
}

func newFakeKBStore() *fakeKBStore { return &fakeKBStore{items: map[uuid.UUID]*data.KnowledgeBase{}} }

func (f *fakeKBStore) Create(ctx context.Context, in data.KBCreate) (*data.KnowledgeBase, error) {
	for _, kb := range f.items {
		if kb.WorkspaceRef == in.WorkspaceRef && kb.Name == in.Name {
			return nil, data.ErrKBDuplicateName
		}
	}
	kb := &data.KnowledgeBase{
		ID:              uuid.New(),
		AccountID:       in.AccountID,
		WorkspaceRef:    in.WorkspaceRef,
		Name:            in.Name,
		Description:     in.Description,
		IntegrationMode: "standalone",
	}
	f.items[kb.ID] = kb
	return kb, nil
}

func (f *fakeKBStore) GetByID(ctx context.Context, id uuid.UUID) (*data.KnowledgeBase, error) {
	return f.items[id], nil
}

func (f *fakeKBStore) ListByWorkspace(ctx context.Context, accountID uuid.UUID, ws string) ([]data.KnowledgeBase, error) {
	var out []data.KnowledgeBase
	for _, kb := range f.items {
		if kb.AccountID == accountID && kb.WorkspaceRef == ws {
			out = append(out, *kb)
		}
	}
	return out, nil
}

func (f *fakeKBStore) Delete(ctx context.Context, id uuid.UUID) error {
	if _, ok := f.items[id]; !ok {
		return data.ErrKBNotFound
	}
	delete(f.items, id)
	return nil
}

type fakeMembership struct{ allow bool }

func (f *fakeMembership) IsWorkspaceMember(ctx context.Context, accountID uuid.UUID, ws string) (bool, error) {
	return f.allow, nil
}

func newHandlerCtx(allow bool) *handlerCtx {
	return &handlerCtx{
		kbStore:    newFakeKBStore(),
		membership: &fakeMembership{allow: allow},
	}
}

func TestCreateKBHappyPath(t *testing.T) {
	ctx := newHandlerCtx(true)
	body := strings.NewReader(`{"name":"my-kb","workspace_ref":"ws-1","description":"desc"}`)
	req := httptest.NewRequest("POST", "/v1/knowledge-bases", body)
	req = injectActor(req, uuid.New(), uuid.New())
	w := httptest.NewRecorder()
	handleCreateKB(ctx)(w, req)
	if w.Code != 201 {
		t.Fatalf("got %d, body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		ID           string `json:"id"`
		Name         string `json:"name"`
		WorkspaceRef string `json:"workspace_ref"`
	}
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp.ID == "" || resp.Name != "my-kb" || resp.WorkspaceRef != "ws-1" {
		t.Errorf("got %+v", resp)
	}
}

func TestCreateKBRejectsNonMember(t *testing.T) {
	ctx := newHandlerCtx(false)
	body := strings.NewReader(`{"name":"x","workspace_ref":"ws-1"}`)
	req := httptest.NewRequest("POST", "/v1/knowledge-bases", body)
	req = injectActor(req, uuid.New(), uuid.New())
	w := httptest.NewRecorder()
	handleCreateKB(ctx)(w, req)
	if w.Code != 403 {
		t.Errorf("got %d", w.Code)
	}
}

func TestCreateKBRejectsBadJSON(t *testing.T) {
	ctx := newHandlerCtx(true)
	req := httptest.NewRequest("POST", "/v1/knowledge-bases", strings.NewReader("not json"))
	req = injectActor(req, uuid.New(), uuid.New())
	w := httptest.NewRecorder()
	handleCreateKB(ctx)(w, req)
	if w.Code != 400 {
		t.Errorf("got %d", w.Code)
	}
}

func TestCreateKBRejectsDuplicateName(t *testing.T) {
	ctx := newHandlerCtx(true)
	for i, body := range []*strings.Reader{
		strings.NewReader(`{"name":"dup","workspace_ref":"ws"}`),
		strings.NewReader(`{"name":"dup","workspace_ref":"ws"}`),
	} {
		req := httptest.NewRequest("POST", "/v1/knowledge-bases", body)
		req = injectActor(req, uuid.New(), uuid.New())
		w := httptest.NewRecorder()
		handleCreateKB(ctx)(w, req)
		if i == 0 && w.Code != 201 {
			t.Fatalf("first should succeed, got %d", w.Code)
		}
		if i == 1 && w.Code != 409 {
			t.Errorf("second should 409, got %d", w.Code)
		}
	}
}

func TestListKBs(t *testing.T) {
	ctx := newHandlerCtx(true)
	acc := uuid.New()
	for _, name := range []string{"a", "b"} {
		_, _ = ctx.kbStore.Create(context.Background(), data.KBCreate{AccountID: acc, WorkspaceRef: "ws", Name: name})
	}
	req := httptest.NewRequest("GET", "/v1/knowledge-bases?workspace_ref=ws", nil)
	req = injectActor(req, acc, uuid.New())
	w := httptest.NewRecorder()
	handleListKB(ctx)(w, req)
	if w.Code != 200 {
		t.Fatalf("got %d, body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Items []map[string]any `json:"items"`
	}
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Items) != 2 {
		t.Errorf("got %d items, want 2", len(resp.Items))
	}
}

func TestDeleteKB(t *testing.T) {
	ctx := newHandlerCtx(true)
	kb, _ := ctx.kbStore.Create(context.Background(), data.KBCreate{AccountID: uuid.New(), WorkspaceRef: "ws", Name: "k"})
	req := httptest.NewRequest("DELETE", "/v1/knowledge-bases/"+kb.ID.String(), nil)
	req.SetPathValue("id", kb.ID.String())
	req = injectActor(req, kb.AccountID, uuid.New())
	w := httptest.NewRecorder()
	handleDeleteKB(ctx)(w, req)
	if w.Code != 204 {
		t.Errorf("got %d", w.Code)
	}
	if got, _ := ctx.kbStore.GetByID(context.Background(), kb.ID); got != nil {
		t.Error("expected nil after delete")
	}
}

// System banks ("组卷题库") are owned by the worker's paper-builder flow,
// not by users. They must be invisible to the per-KB REST routes (GET,
// DELETE, document upload/list/get/delete, search) so admins cannot
// accidentally browse, mutate, or delete them via console-lite. The
// pattern is: loadAuthorizedKB returns 404 if kb.Kind == 'system_paper_bank'.
func TestPerKBRoutesHideSystemBank(t *testing.T) {
	ctx := newHandlerCtx(true)
	acc := uuid.New()
	user := uuid.New()
	bank := &data.KnowledgeBase{
		ID:           uuid.New(),
		AccountID:    acc,
		WorkspaceRef: "ws",
		Name:         "组卷题库",
		Kind:         data.KBKindSystemPaperBank,
		Visibility:   "workspace_member",
	}
	store := ctx.kbStore.(*fakeKBStore)
	store.items[bank.ID] = bank

	cases := []struct {
		name    string
		method  string
		handler nethttp.HandlerFunc
	}{
		{"GET", "GET", handleGetKB(ctx)},
		{"DELETE", "DELETE", handleDeleteKB(ctx)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "/v1/knowledge-bases/"+bank.ID.String(), nil)
			req.SetPathValue("id", bank.ID.String())
			req = injectActor(req, acc, user)
			w := httptest.NewRecorder()
			tc.handler(w, req)
			if w.Code != 404 {
				t.Errorf("expected 404 for system bank, got %d, body=%s", w.Code, w.Body.String())
			}
		})
	}
	// And the bank should still exist after the failed DELETE attempt.
	if got, _ := store.GetByID(context.Background(), bank.ID); got == nil {
		t.Error("system bank must not be deleted via REST")
	}
}

type fakeExamTokenSource struct {
	token  string
	userID uuid.UUID
	scopes []string
}

func (f *fakeExamTokenSource) IssueExamToken(ctx context.Context, userID uuid.UUID, scopes []string) (string, error) {
	f.userID = userID
	f.scopes = append([]string(nil), scopes...)
	return f.token, nil
}

type fakeExamScopesLister struct {
	gotToken string
}

func (f *fakeExamScopesLister) ListExamScopes(ctx context.Context, token string) ([]map[string]any, error) {
	f.gotToken = token
	return []map[string]any{{"id": "scope-1", "name": "Scope 1"}}, nil
}

func TestHandleExamScopes_MintsExamTokenForCurrentActor(t *testing.T) {
	userID := uuid.New()
	tokenSource := &fakeExamTokenSource{token: "exam-token"}
	lister := &fakeExamScopesLister{}
	ctx := &handlerCtx{
		examScopesLister: lister,
		examTokenSource:  tokenSource,
	}

	req := httptest.NewRequest(nethttp.MethodGet, "/v1/exam/scopes", nil)
	req.Header.Set("Authorization", "Bearer arkloop-token")
	req = injectActor(req, uuid.New(), userID)
	w := httptest.NewRecorder()

	handleExamScopes(ctx)(w, req)

	if w.Code != nethttp.StatusOK {
		t.Fatalf("got %d, body=%s", w.Code, w.Body.String())
	}
	if lister.gotToken != "exam-token" {
		t.Fatalf("lister got token %q, want minted exam token", lister.gotToken)
	}
	if tokenSource.userID != userID {
		t.Fatalf("token source got user %s, want %s", tokenSource.userID, userID)
	}
	if got := strings.Join(tokenSource.scopes, " "); got != "openid exam:read" {
		t.Fatalf("token source scopes %q, want openid exam:read", got)
	}
}

func TestHandleKnowledgeBaseScopes_MintsExamTokenInternally(t *testing.T) {
	userID := uuid.New()
	tokenSource := &fakeExamTokenSource{token: "exam-token"}
	lister := &fakeExamScopesLister{}
	ctx := &handlerCtx{
		examScopesLister: lister,
		examTokenSource:  tokenSource,
	}

	req := httptest.NewRequest(nethttp.MethodGet, "/v1/knowledge-bases/scopes", nil)
	req = injectActor(req, uuid.New(), userID)
	w := httptest.NewRecorder()

	handleKnowledgeBaseScopes(ctx)(w, req)

	if w.Code != nethttp.StatusOK {
		t.Fatalf("got %d, body=%s", w.Code, w.Body.String())
	}
	if lister.gotToken != "exam-token" {
		t.Fatalf("lister got token %q, want minted exam token", lister.gotToken)
	}
}
