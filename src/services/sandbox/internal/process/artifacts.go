package process

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"

	"arkloop/services/sandbox/internal/logging"
	"arkloop/services/sandbox/internal/session"
	"arkloop/services/shared/objectstore"
	"github.com/google/uuid"
)

type artifactVersion struct {
	Size     int64
	SHA256   string
	MimeType string
}

type artifactUploadResult struct {
	Refs      []ArtifactRef
	NextKnown map[string]artifactVersion
}

type artifactStore interface {
	PutObject(ctx context.Context, key string, data []byte, options objectstore.PutOptions) error
}

func collectArtifacts(
	ctx context.Context,
	sn *session.Session,
	sessionID string,
	commandSeq int64,
	store artifactStore,
	known map[string]artifactVersion,
	logger *logging.JSONLogger,
) artifactUploadResult {
	nextKnown := cloneArtifactVersions(known)
	if store == nil || sn == nil {
		return artifactUploadResult{NextKnown: nextKnown}
	}
	fetchResult, err := sn.FetchArtifacts(ctx)
	if err != nil {
		if logger != nil {
			logger.Warn("fetch process artifacts failed", logging.LogFields{SessionID: &sessionID}, map[string]any{"error": err.Error()})
		}
		return artifactUploadResult{NextKnown: nextKnown}
	}
	refs := make([]ArtifactRef, 0, len(fetchResult.Artifacts))
	seen := make(map[string]struct{}, len(fetchResult.Artifacts))
	freshKnown := make(map[string]artifactVersion, len(fetchResult.Artifacts))
	for _, entry := range fetchResult.Artifacts {
		name := filepath.Base(entry.Filename)
		if name == "" || name == "." || name == ".." {
			continue
		}
		seen[name] = struct{}{}
		data, err := base64.StdEncoding.DecodeString(entry.Data)
		if err != nil {
			continue
		}
		version := newArtifactVersion(data, entry.MimeType)
		freshKnown[name] = version
		if current, ok := nextKnown[name]; ok && current == version {
			continue
		}
		key := artifactObjectKey(sn.AccountID, sessionID, commandSeq, name)
		metadata := objectstore.ArtifactMetadata(objectstore.ArtifactOwnerKindRun, resolveArtifactOwnerRunID(sessionID), sn.AccountID, nil)
		if err := store.PutObject(ctx, key, data, objectstore.PutOptions{ContentType: entry.MimeType, Metadata: metadata}); err != nil {
			continue
		}
		nextKnown[name] = version
		refs = append(refs, ArtifactRef{
			Key:      key,
			Filename: name,
			Size:     version.Size,
			MimeType: entry.MimeType,
		})
	}
	for name, version := range nextKnown {
		if _, ok := seen[name]; ok {
			continue
		}
		freshKnown[name] = version
	}
	return artifactUploadResult{Refs: refs, NextKnown: freshKnown}
}

func cloneArtifactVersions(source map[string]artifactVersion) map[string]artifactVersion {
	if len(source) == 0 {
		return map[string]artifactVersion{}
	}
	out := make(map[string]artifactVersion, len(source))
	for key, value := range source {
		out[key] = value
	}
	return out
}

func newArtifactVersion(data []byte, mimeType string) artifactVersion {
	sum := sha256.Sum256(data)
	return artifactVersion{
		Size:     int64(len(data)),
		SHA256:   hex.EncodeToString(sum[:]),
		MimeType: mimeType,
	}
}

func artifactObjectKey(accountID, sessionID string, commandSeq int64, filename string) string {
	return fmt.Sprintf("%s/%s/%d/%s", accountID, sessionID, commandSeq, filename)
}

func resolveArtifactOwnerRunID(sessionID string) string {
	if sessionID == "" {
		return ""
	}
	parts := strings.Split(sessionID, "/")
	if len(parts) == 0 {
		return sessionID
	}
	if _, err := uuid.Parse(parts[0]); err == nil {
		return parts[0]
	}
	return sessionID
}
