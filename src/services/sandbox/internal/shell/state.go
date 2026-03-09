package shell

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"arkloop/services/shared/objectstore"
)

const (
	shellStateVersion = 1
	defaultRestoreCwd = "/workspace"
)

type stateStore interface {
	Put(ctx context.Context, key string, data []byte) error
	Get(ctx context.Context, key string) ([]byte, error)
}

var _ stateStore = (*objectstore.S3Store)(nil)

type SessionRestoreState struct {
	Version        int                        `json:"version"`
	Revision       string                     `json:"revision"`
	OrgID          string                     `json:"org_id"`
	SessionID      string                     `json:"session_ref"`
	ProfileRef     string                     `json:"profile_ref,omitempty"`
	WorkspaceRef   string                     `json:"workspace_ref,omitempty"`
	Cwd            string                     `json:"cwd"`
	EnvSnapshot    map[string]string          `json:"env_snapshot,omitempty"`
	LastCommandSeq int64                      `json:"last_command_seq"`
	UploadedSeq    int64                      `json:"uploaded_seq"`
	ArtifactSeen   map[string]artifactVersion `json:"artifact_seen,omitempty"`
	CreatedAt      string                     `json:"created_at"`
	ExpiresAt      string                     `json:"expires_at,omitempty"`
}

func nextRestoreRevision(now time.Time) string {
	return fmt.Sprintf("%d", now.UTC().UnixNano())
}

func sessionRestoreStateKey(sessionID, revision string) string {
	return "sessions/" + strings.TrimSpace(sessionID) + "/restore/" + strings.TrimSpace(revision) + ".json"
}

func saveRestoreState(ctx context.Context, store stateStore, registry SessionRestoreRegistry, state SessionRestoreState) error {
	if store == nil {
		return fmt.Errorf("restore state store is required")
	}
	state.Revision = strings.TrimSpace(state.Revision)
	if state.Revision == "" {
		return fmt.Errorf("restore revision must not be empty")
	}
	state.SessionID = strings.TrimSpace(state.SessionID)
	if state.SessionID == "" {
		return fmt.Errorf("session_ref must not be empty")
	}
	state.OrgID = strings.TrimSpace(state.OrgID)
	if state.OrgID == "" {
		return fmt.Errorf("org_id must not be empty")
	}
	state.Cwd = strings.TrimSpace(state.Cwd)
	if state.Cwd == "" {
		state.Cwd = defaultRestoreCwd
	}
	state.ProfileRef = strings.TrimSpace(state.ProfileRef)
	state.WorkspaceRef = strings.TrimSpace(state.WorkspaceRef)
	normalizeArtifactVersions(state.ArtifactSeen)

	payload, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal restore state: %w", err)
	}
	if err := store.Put(ctx, sessionRestoreStateKey(state.SessionID, state.Revision), payload); err != nil {
		return fmt.Errorf("put restore state: %w", err)
	}
	if registry == nil {
		return nil
	}
	if err := registry.BindLatestRestoreRevision(ctx, state.OrgID, state.SessionID, state.Revision); err != nil {
		return fmt.Errorf("bind restore revision: %w", err)
	}
	return nil
}

func loadLatestRestoreState(ctx context.Context, store stateStore, registry SessionRestoreRegistry, orgID, sessionID string) (*SessionRestoreState, error) {
	if store == nil || registry == nil {
		return nil, os.ErrNotExist
	}
	revision, err := registry.GetLatestRestoreRevision(ctx, orgID, sessionID)
	if err != nil {
		return nil, err
	}
	revision = strings.TrimSpace(revision)
	if revision == "" {
		return nil, os.ErrNotExist
	}
	payload, err := store.Get(ctx, sessionRestoreStateKey(sessionID, revision))
	if err != nil {
		return nil, err
	}
	var state SessionRestoreState
	if err := json.Unmarshal(payload, &state); err != nil {
		return nil, fmt.Errorf("decode restore state: %w", err)
	}
	if state.Version != shellStateVersion {
		return nil, fmt.Errorf("unsupported restore state version: %d", state.Version)
	}
	if strings.TrimSpace(state.OrgID) != strings.TrimSpace(orgID) || strings.TrimSpace(state.SessionID) != strings.TrimSpace(sessionID) {
		return nil, fmt.Errorf("restore state identity mismatch")
	}
	if strings.TrimSpace(state.Cwd) == "" {
		state.Cwd = defaultRestoreCwd
	}
	normalizeArtifactVersions(state.ArtifactSeen)
	return &state, nil
}

func normalizeArtifactVersions(versions map[string]artifactVersion) {
	for name, version := range versions {
		if version.SHA256 == "" && strings.TrimSpace(version.Data) != "" {
			if decoded, err := base64.StdEncoding.DecodeString(version.Data); err == nil {
				normalized := newArtifactVersion(decoded, version.MimeType)
				version.Size = normalized.Size
				version.SHA256 = normalized.SHA256
			}
		}
		version.Data = ""
		versions[name] = version
	}
}

func copyLatestRestoreState(ctx context.Context, store stateStore, registry SessionRestoreRegistry, orgID, fromSessionID, toSessionID string) (string, error) {
	state, err := loadLatestRestoreState(ctx, store, registry, orgID, fromSessionID)
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	copied := *state
	copied.SessionID = strings.TrimSpace(toSessionID)
	copied.Revision = nextRestoreRevision(now)
	copied.CreatedAt = now.Format(time.RFC3339Nano)
	copied.ArtifactSeen = cloneArtifactSeen(state.ArtifactSeen)
	if err := saveRestoreState(ctx, store, registry, copied); err != nil {
		return "", err
	}
	return copied.Revision, nil
}
