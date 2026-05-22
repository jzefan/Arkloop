package kbapi

import nethttp "net/http"

// Register wires KB routes onto mux. Actor injection is applied by the caller.
func Register(mux *nethttp.ServeMux, h *handlerCtx) {
	mux.Handle("POST /v1/knowledge-bases", handleCreateKB(h))
	mux.Handle("GET /v1/knowledge-bases", handleListKB(h))
	mux.Handle("GET /v1/knowledge-bases/{id}", handleGetKB(h))
	mux.Handle("DELETE /v1/knowledge-bases/{id}", handleDeleteKB(h))
	mux.Handle("POST /v1/knowledge-bases/{id}/documents", handleUploadDoc(h))
	mux.Handle("GET /v1/knowledge-bases/{id}/documents", handleListDocs(h))
	mux.Handle("GET /v1/knowledge-bases/{id}/documents/{doc_id}", handleGetDoc(h))
	mux.Handle("DELETE /v1/knowledge-bases/{id}/documents/{doc_id}", handleDeleteDoc(h))
	mux.Handle("GET /v1/knowledge-bases/{id}/search", handleSearch(h))
}
