package kbapi

import nethttp "net/http"

// Register wires KB routes onto mux and resolves actor auth before each route.
func Register(mux *nethttp.ServeMux, h *handlerCtx) {
	mux.Handle("POST /v1/knowledge-bases", h.withActor(handleCreateKB(h)))
	mux.Handle("GET /v1/knowledge-bases", h.withActor(handleListKB(h)))
	mux.Handle("GET /v1/knowledge-bases/{id}", h.withActor(handleGetKB(h)))
	mux.Handle("DELETE /v1/knowledge-bases/{id}", h.withActor(handleDeleteKB(h)))
	mux.Handle("POST /v1/knowledge-bases/{id}/documents", h.withActor(handleUploadDoc(h)))
	mux.Handle("GET /v1/knowledge-bases/{id}/documents", h.withActor(handleListDocs(h)))
	mux.Handle("GET /v1/knowledge-bases/{id}/documents/{doc_id}", h.withActor(handleGetDoc(h)))
	mux.Handle("DELETE /v1/knowledge-bases/{id}/documents/{doc_id}", h.withActor(handleDeleteDoc(h)))
	mux.Handle("GET /v1/knowledge-bases/{id}/search", h.withActor(handleSearch(h)))
}
