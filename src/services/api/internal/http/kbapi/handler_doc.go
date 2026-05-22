package kbapi

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	nethttp "net/http"
	"path/filepath"
	"strings"

	"arkloop/services/api/internal/data"

	"github.com/google/uuid"
)

const defaultMaxUploadBytes int64 = 10 * 1024 * 1024

func handleUploadDoc(h *handlerCtx) nethttp.HandlerFunc {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		a, ok := actorFromCtx(r.Context())
		if !ok {
			writeErr(w, nethttp.StatusUnauthorized, "auth.unauthenticated", "unauthenticated")
			return
		}
		kb, ok := loadAuthorizedKB(w, r, h, a)
		if !ok {
			return
		}
		maxBytes := h.maxUploadBytes
		if maxBytes == 0 {
			maxBytes = defaultMaxUploadBytes
		}
		r.Body = nethttp.MaxBytesReader(w, r.Body, maxBytes)
		if err := r.ParseMultipartForm(maxBytes); err != nil {
			var maxErr *nethttp.MaxBytesError
			if errors.As(err, &maxErr) {
				writeErr(w, nethttp.StatusRequestEntityTooLarge, "kb.upload_too_large", "uploaded file exceeds 10MB limit")
				return
			}
			writeErr(w, nethttp.StatusBadRequest, "kb.bad_multipart", "could not parse multipart body")
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			writeErr(w, nethttp.StatusBadRequest, "kb.missing_file", "form field 'file' is required")
			return
		}
		defer file.Close()

		ext := strings.ToLower(filepath.Ext(header.Filename))
		mime := mimeFromExt(ext)
		if mime == "" {
			writeErr(w, nethttp.StatusUnsupportedMediaType, "kb.unsupported_format", "only .txt and .md are supported in M1.0")
			return
		}
		buf := &bytes.Buffer{}
		n, err := io.Copy(buf, file)
		if err != nil {
			writeErr(w, nethttp.StatusBadRequest, "kb.read_failed", "could not read uploaded file")
			return
		}
		sum := sha256.Sum256(buf.Bytes())
		shaHex := hex.EncodeToString(sum[:])
		if err := h.blob.PutBlob(r.Context(), kb.WorkspaceRef, shaHex, buf.Bytes()); err != nil {
			writeErr(w, nethttp.StatusInternalServerError, "internal.blob_failed", "failed to persist blob")
			return
		}
		doc, err := h.docStore.Create(r.Context(), data.DocCreate{
			KBID:             kb.ID,
			OriginalFilename: header.Filename,
			MimeType:         mime,
			BlobSHA256:       shaHex,
			SizeBytes:        n,
			CreatedBy:        &a.UserID,
		})
		if err != nil {
			writeErr(w, nethttp.StatusInternalServerError, "internal.doc_create_failed", "failed to record document")
			return
		}
		jobID, err := h.jobs.EnqueueKBIngest(r.Context(), a.AccountID, kb.ID, doc.ID, kb.WorkspaceRef, shaHex, mime, header.Filename, "")
		if err != nil {
			writeErr(w, nethttp.StatusInternalServerError, "internal.enqueue_failed", "failed to enqueue ingest job")
			return
		}
		writeJSON(w, nethttp.StatusCreated, map[string]any{"document_id": doc.ID, "job_id": jobID})
	}
}

func handleListDocs(h *handlerCtx) nethttp.HandlerFunc {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		a, ok := actorFromCtx(r.Context())
		if !ok {
			writeErr(w, nethttp.StatusUnauthorized, "auth.unauthenticated", "unauthenticated")
			return
		}
		kb, ok := loadAuthorizedKB(w, r, h, a)
		if !ok {
			return
		}
		docs, err := h.docStore.ListByKB(r.Context(), kb.ID)
		if err != nil {
			writeErr(w, nethttp.StatusInternalServerError, "internal.error", "list failed")
			return
		}
		items := make([]map[string]any, 0, len(docs))
		for i := range docs {
			items = append(items, docResponse(&docs[i]))
		}
		writeJSON(w, nethttp.StatusOK, map[string]any{"items": items})
	}
}

func handleGetDoc(h *handlerCtx) nethttp.HandlerFunc {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		a, ok := actorFromCtx(r.Context())
		if !ok {
			writeErr(w, nethttp.StatusUnauthorized, "auth.unauthenticated", "unauthenticated")
			return
		}
		kb, ok := loadAuthorizedKB(w, r, h, a)
		if !ok {
			return
		}
		doc, ok := loadDoc(w, r, h, kb.ID)
		if !ok {
			return
		}
		writeJSON(w, nethttp.StatusOK, docResponse(doc))
	}
}

func handleDeleteDoc(h *handlerCtx) nethttp.HandlerFunc {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		a, ok := actorFromCtx(r.Context())
		if !ok {
			writeErr(w, nethttp.StatusUnauthorized, "auth.unauthenticated", "unauthenticated")
			return
		}
		kb, ok := loadAuthorizedKB(w, r, h, a)
		if !ok {
			return
		}
		doc, ok := loadDoc(w, r, h, kb.ID)
		if !ok {
			return
		}
		if err := h.docStore.Delete(r.Context(), doc.ID); err != nil {
			if errors.Is(err, data.ErrDocNotFound) {
				writeErr(w, nethttp.StatusNotFound, "kb.doc_not_found", "document not found")
				return
			}
			writeErr(w, nethttp.StatusInternalServerError, "internal.error", "delete failed")
			return
		}
		w.WriteHeader(nethttp.StatusNoContent)
	}
}

func loadDoc(w nethttp.ResponseWriter, r *nethttp.Request, h *handlerCtx, kbID uuid.UUID) (*data.KBDocument, bool) {
	docID, err := uuid.Parse(r.PathValue("doc_id"))
	if err != nil {
		writeErr(w, nethttp.StatusBadRequest, "kb.invalid_doc_id", "invalid doc id")
		return nil, false
	}
	doc, err := h.docStore.GetByID(r.Context(), docID)
	if err != nil || doc == nil || doc.KBID != kbID {
		writeErr(w, nethttp.StatusNotFound, "kb.doc_not_found", "document not found")
		return nil, false
	}
	return doc, true
}

func docResponse(doc *data.KBDocument) map[string]any {
	return map[string]any{
		"id":                doc.ID,
		"original_filename": doc.OriginalFilename,
		"mime_type":         doc.MimeType,
		"size_bytes":        doc.SizeBytes,
		"status":            doc.Status,
		"error_message":     doc.ErrorMessage,
		"parse_meta":        doc.ParseMeta,
		"created_at":        doc.CreatedAt,
		"updated_at":        doc.UpdatedAt,
	}
}

func mimeFromExt(ext string) string {
	switch ext {
	case ".txt":
		return "text/plain"
	case ".md":
		return "text/markdown"
	default:
		return ""
	}
}
