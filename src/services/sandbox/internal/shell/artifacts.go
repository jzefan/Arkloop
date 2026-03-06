package shell

import (
	"context"
	"encoding/base64"
	"fmt"
	"path/filepath"

	"arkloop/services/sandbox/internal/logging"
	"arkloop/services/sandbox/internal/session"
	"arkloop/services/shared/objectstore"
)

type artifactVersion struct {
	Size int64
	Data string
}

func collectArtifacts(
	ctx context.Context,
	sn *session.Session,
	sessionID string,
	commandSeq int64,
	store artifactStore,
	known map[string]artifactVersion,
	logger *logging.JSONLogger,
) ([]ArtifactRef, map[string]artifactVersion) {
	if store == nil {
		return nil, known
	}

	fetchResult, err := sn.FetchArtifacts(ctx)
	if err != nil {
		logger.Warn("fetch shell artifacts failed", logging.LogFields{SessionID: &sessionID}, map[string]any{"error": err.Error()})
		return nil, known
	}

	nextKnown := make(map[string]artifactVersion, len(fetchResult.Artifacts))
	refs := make([]ArtifactRef, 0, len(fetchResult.Artifacts))
	for _, entry := range fetchResult.Artifacts {
		safeName := filepath.Base(entry.Filename)
		if safeName == "." || safeName == ".." || safeName == "" {
			continue
		}

		version := artifactVersion{Size: entry.Size, Data: entry.Data}
		nextKnown[safeName] = version
		if current, ok := known[safeName]; ok && current == version {
			continue
		}

		data, err := base64.StdEncoding.DecodeString(entry.Data)
		if err != nil {
			logger.Warn("decode shell artifact failed", logging.LogFields{SessionID: &sessionID}, map[string]any{"filename": safeName, "error": err.Error()})
			continue
		}

		key := fmt.Sprintf("%s/%s/shell/%d/%s", sn.OrgID, sessionID, commandSeq, safeName)
		if err := store.PutWithContentType(ctx, key, data, entry.MimeType); err != nil {
			logger.Warn("upload shell artifact failed", logging.LogFields{SessionID: &sessionID}, map[string]any{"key": key, "error": err.Error()})
			continue
		}

		refs = append(refs, ArtifactRef{
			Key:      key,
			Filename: safeName,
			Size:     entry.Size,
			MimeType: entry.MimeType,
		})
	}

	return refs, nextKnown
}

type artifactStore interface {
	PutWithContentType(ctx context.Context, key string, data []byte, contentType string) error
}

var _ artifactStore = (*objectstore.Store)(nil)
