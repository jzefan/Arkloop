package kbapi

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http/httptest"
	"testing"

	"arkloop/services/api/internal/data"

	"github.com/google/uuid"
)

type fakeDocStore struct {
	items map[uuid.UUID]*data.KBDocument
}

func newFakeDocStore() *fakeDocStore { return &fakeDocStore{items: map[uuid.UUID]*data.KBDocument{}} }

func (f *fakeDocStore) Create(ctx context.Context, in data.DocCreate) (*data.KBDocument, error) {
	d := &data.KBDocument{
		ID:               uuid.New(),
		KBID:             in.KBID,
		OriginalFilename: in.OriginalFilename,
		MimeType:         in.MimeType,
		BlobSHA256:       in.BlobSHA256,
		SizeBytes:        in.SizeBytes,
		Status:           "queued",
		ParseMeta:        map[string]any{},
	}
	f.items[d.ID] = d
	return d, nil
}

func (f *fakeDocStore) GetByID(ctx context.Context, id uuid.UUID) (*data.KBDocument, error) {
	return f.items[id], nil
}

func (f *fakeDocStore) ListByKB(ctx context.Context, kbID uuid.UUID) ([]data.KBDocument, error) {
	var out []data.KBDocument
	for _, d := range f.items {
		if d.KBID == kbID {
			out = append(out, *d)
		}
	}
	return out, nil
}

func (f *fakeDocStore) Delete(ctx context.Context, id uuid.UUID) error {
	if _, ok := f.items[id]; !ok {
		return data.ErrDocNotFound
	}
	delete(f.items, id)
	return nil
}

func (f *fakeDocStore) CountByBlobSHA256(ctx context.Context, sha string) (int, error) {
	n := 0
	for _, d := range f.items {
		if d.BlobSHA256 == sha {
			n++
		}
	}
	return n, nil
}

type fakeBlobStore struct {
	puts map[string][]byte
}

func newFakeBlobStore() *fakeBlobStore { return &fakeBlobStore{puts: map[string][]byte{}} }

func (b *fakeBlobStore) PutBlob(ctx context.Context, workspaceRef, sha256 string, data []byte) error {
	b.puts[workspaceRef+"/"+sha256] = append([]byte(nil), data...)
	return nil
}

func (b *fakeBlobStore) DeleteBlob(ctx context.Context, workspaceRef, sha256 string) error {
	delete(b.puts, workspaceRef+"/"+sha256)
	return nil
}

type fakeJobEnqueue struct {
	called int
	mimes  []string
}

func (q *fakeJobEnqueue) EnqueueKBIngest(ctx context.Context, accountID, kbID, docID uuid.UUID, workspaceRef, blobSHA256, mimeType, filename, traceID string) (uuid.UUID, error) {
	q.called++
	q.mimes = append(q.mimes, mimeType)
	return uuid.New(), nil
}

func newDocCtx(allow bool) (*handlerCtx, *fakeDocStore, *fakeBlobStore, *fakeJobEnqueue, *fakeKBStore) {
	kbStore := newFakeKBStore()
	docStore := newFakeDocStore()
	blob := newFakeBlobStore()
	jobs := &fakeJobEnqueue{}
	ctx := &handlerCtx{
		kbStore:    kbStore,
		docStore:   docStore,
		membership: &fakeMembership{allow: allow},
		blob:       blob,
		jobs:       jobs,
	}
	return ctx, docStore, blob, jobs, kbStore
}

func buildMultipart(t *testing.T, filename string, body []byte) (*bytes.Buffer, string) {
	t.Helper()
	buf := &bytes.Buffer{}
	w := multipart.NewWriter(buf)
	fw, err := w.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("form file: %v", err)
	}
	_, _ = fw.Write(body)
	_ = w.Close()
	return buf, w.FormDataContentType()
}

func TestUploadDocHappyPath(t *testing.T) {
	ctx, docs, blob, jobs, kbStore := newDocCtx(true)
	acc := uuid.New()
	kb, _ := kbStore.Create(context.Background(), data.KBCreate{AccountID: acc, WorkspaceRef: "ws", Name: "n"})

	body, ctType := buildMultipart(t, "a.txt", []byte("hello world"))
	req := httptest.NewRequest("POST", "/v1/knowledge-bases/"+kb.ID.String()+"/documents", body)
	req.Header.Set("Content-Type", ctType)
	req.SetPathValue("id", kb.ID.String())
	req = injectActor(req, acc, uuid.New())
	w := httptest.NewRecorder()
	handleUploadDoc(ctx)(w, req)
	if w.Code != 201 {
		t.Fatalf("got %d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		DocumentID string `json:"document_id"`
		JobID      string `json:"job_id"`
	}
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp.DocumentID == "" || resp.JobID == "" {
		t.Errorf("missing ids: %+v", resp)
	}
	if len(docs.items) != 1 {
		t.Errorf("docs not persisted: %d", len(docs.items))
	}
	if len(blob.puts) != 1 {
		t.Errorf("blob not written: %d", len(blob.puts))
	}
	if jobs.called != 1 {
		t.Errorf("job not enqueued: %d", jobs.called)
	}
}

func TestUploadDocRejectsOversize(t *testing.T) {
	ctx, _, _, _, kbStore := newDocCtx(true)
	ctx.maxUploadBytes = 16
	acc := uuid.New()
	kb, _ := kbStore.Create(context.Background(), data.KBCreate{AccountID: acc, WorkspaceRef: "ws", Name: "n"})
	body, ctType := buildMultipart(t, "big.txt", bytes.Repeat([]byte("x"), 1024))
	req := httptest.NewRequest("POST", "/v1/knowledge-bases/"+kb.ID.String()+"/documents", body)
	req.Header.Set("Content-Type", ctType)
	req.SetPathValue("id", kb.ID.String())
	req = injectActor(req, acc, uuid.New())
	w := httptest.NewRecorder()
	handleUploadDoc(ctx)(w, req)
	if w.Code != 413 {
		t.Errorf("got %d, want 413", w.Code)
	}
}

func TestUploadDocAcceptsM11Formats(t *testing.T) {
	cases := []struct {
		filename string
		mime     string
	}{
		{"a.pdf", "application/pdf"},
		{"a.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
		{"a.png", "image/png"},
		{"a.jpg", "image/jpeg"},
		{"a.jpeg", "image/jpeg"},
		{"a.webp", "image/webp"},
	}
	for _, tc := range cases {
		t.Run(tc.filename, func(t *testing.T) {
			ctx, docs, blob, jobs, kbStore := newDocCtx(true)
			acc := uuid.New()
			kb, _ := kbStore.Create(context.Background(), data.KBCreate{AccountID: acc, WorkspaceRef: "ws", Name: "n"})
			body, ctType := buildMultipart(t, tc.filename, []byte("payload"))
			req := httptest.NewRequest("POST", "/v1/knowledge-bases/"+kb.ID.String()+"/documents", body)
			req.Header.Set("Content-Type", ctType)
			req.SetPathValue("id", kb.ID.String())
			req = injectActor(req, acc, uuid.New())
			w := httptest.NewRecorder()
			handleUploadDoc(ctx)(w, req)
			if w.Code != 201 {
				t.Fatalf("got %d body=%s", w.Code, w.Body.String())
			}
			if len(docs.items) != 1 || len(blob.puts) != 1 || jobs.called != 1 {
				t.Fatalf("unexpected side effects docs=%d blobs=%d jobs=%d", len(docs.items), len(blob.puts), jobs.called)
			}
			if jobs.mimes[0] != tc.mime {
				t.Fatalf("mime got %q, want %q", jobs.mimes[0], tc.mime)
			}
		})
	}
}

func TestUploadDocRejectsUnsupportedExt(t *testing.T) {
	ctx, _, _, _, kbStore := newDocCtx(true)
	acc := uuid.New()
	kb, _ := kbStore.Create(context.Background(), data.KBCreate{AccountID: acc, WorkspaceRef: "ws", Name: "n"})
	body, ctType := buildMultipart(t, "a.zip", []byte("PK"))
	req := httptest.NewRequest("POST", "/v1/knowledge-bases/"+kb.ID.String()+"/documents", body)
	req.Header.Set("Content-Type", ctType)
	req.SetPathValue("id", kb.ID.String())
	req = injectActor(req, acc, uuid.New())
	w := httptest.NewRecorder()
	handleUploadDoc(ctx)(w, req)
	if w.Code != 415 {
		t.Errorf("got %d, want 415", w.Code)
	}
}

func TestListDocsByKB(t *testing.T) {
	ctx, docs, _, _, kbStore := newDocCtx(true)
	acc := uuid.New()
	kb, _ := kbStore.Create(context.Background(), data.KBCreate{AccountID: acc, WorkspaceRef: "ws", Name: "n"})
	for _, name := range []string{"a.txt", "b.txt"} {
		_, _ = docs.Create(context.Background(), data.DocCreate{KBID: kb.ID, OriginalFilename: name, MimeType: "text/plain", BlobSHA256: "s", SizeBytes: 1})
	}
	req := httptest.NewRequest("GET", "/v1/knowledge-bases/"+kb.ID.String()+"/documents", nil)
	req.SetPathValue("id", kb.ID.String())
	req = injectActor(req, acc, uuid.New())
	w := httptest.NewRecorder()
	handleListDocs(ctx)(w, req)
	if w.Code != 200 {
		t.Fatalf("got %d", w.Code)
	}
	var resp struct {
		Items []map[string]any `json:"items"`
	}
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Items) != 2 {
		t.Errorf("got %d items", len(resp.Items))
	}
}

func TestGetDocStatus(t *testing.T) {
	ctx, docs, _, _, kbStore := newDocCtx(true)
	acc := uuid.New()
	kb, _ := kbStore.Create(context.Background(), data.KBCreate{AccountID: acc, WorkspaceRef: "ws", Name: "n"})
	doc, _ := docs.Create(context.Background(), data.DocCreate{KBID: kb.ID, OriginalFilename: "a.txt", MimeType: "text/plain", BlobSHA256: "s", SizeBytes: 1})
	req := httptest.NewRequest("GET", "/v1/knowledge-bases/"+kb.ID.String()+"/documents/"+doc.ID.String(), nil)
	req.SetPathValue("id", kb.ID.String())
	req.SetPathValue("doc_id", doc.ID.String())
	req = injectActor(req, acc, uuid.New())
	w := httptest.NewRecorder()
	handleGetDoc(ctx)(w, req)
	if w.Code != 200 {
		t.Fatalf("got %d", w.Code)
	}
}

func TestDeleteDoc(t *testing.T) {
	ctx, docs, _, _, kbStore := newDocCtx(true)
	acc := uuid.New()
	kb, _ := kbStore.Create(context.Background(), data.KBCreate{AccountID: acc, WorkspaceRef: "ws", Name: "n"})
	doc, _ := docs.Create(context.Background(), data.DocCreate{KBID: kb.ID, OriginalFilename: "a.txt", MimeType: "text/plain", BlobSHA256: "s", SizeBytes: 1})
	req := httptest.NewRequest("DELETE", "/v1/knowledge-bases/"+kb.ID.String()+"/documents/"+doc.ID.String(), nil)
	req.SetPathValue("id", kb.ID.String())
	req.SetPathValue("doc_id", doc.ID.String())
	req = injectActor(req, acc, uuid.New())
	w := httptest.NewRecorder()
	handleDeleteDoc(ctx)(w, req)
	if w.Code != 204 {
		t.Errorf("got %d", w.Code)
	}
	if got, _ := docs.GetByID(context.Background(), doc.ID); got != nil {
		t.Error("not deleted")
	}
}
