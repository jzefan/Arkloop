package contract

import "strings"

const CurrentVersion = 1

type Manifest struct {
	Version      int             `json:"version"`
	Scope        string          `json:"scope"`
	Ref          string          `json:"ref"`
	Revision     string          `json:"revision"`
	BaseRevision string          `json:"base_revision,omitempty"`
	CreatedAt    string          `json:"created_at"`
	Entries      []ManifestEntry `json:"entries,omitempty"`
	Stats        ManifestStats   `json:"stats,omitempty"`
}

type ManifestEntry struct {
	Path        string `json:"path"`
	Type        string `json:"type"`
	Mode        int64  `json:"mode,omitempty"`
	Size        int64  `json:"size,omitempty"`
	SHA256      string `json:"sha256,omitempty"`
	MtimeUnixMs int64  `json:"mtime_unix_ms,omitempty"`
	LinkTarget  string `json:"link_target,omitempty"`
	Deleted     bool   `json:"deleted,omitempty"`
}

type ManifestStats struct {
	FileCount int64 `json:"file_count,omitempty"`
	DirCount  int64 `json:"dir_count,omitempty"`
	ByteCount int64 `json:"byte_count,omitempty"`
}

type SessionRestoreState struct {
	Version        int               `json:"version"`
	SessionRef     string            `json:"session_ref"`
	ProfileRef     string            `json:"profile_ref"`
	WorkspaceRef   string            `json:"workspace_ref"`
	Cwd            string            `json:"cwd,omitempty"`
	EnvDelta       map[string]string `json:"env_delta,omitempty"`
	LastCommandSeq int64             `json:"last_command_seq,omitempty"`
	CreatedAt      string            `json:"created_at"`
	ExpiresAt      string            `json:"expires_at,omitempty"`
}

func ProfileManifestKey(profileRef string, revision string) string {
	return "profiles/" + strings.TrimSpace(profileRef) + "/manifests/" + strings.TrimSpace(revision) + ".json"
}

func ProfileLatestKey(profileRef string) string {
	return "profiles/" + strings.TrimSpace(profileRef) + "/latest.json"
}

func ProfileBlobKey(profileRef string, sha256 string) string {
	return "profiles/" + strings.TrimSpace(profileRef) + "/blobs/" + strings.TrimSpace(sha256)
}

func WorkspaceManifestKey(workspaceRef string, revision string) string {
	return "workspaces/" + strings.TrimSpace(workspaceRef) + "/manifests/" + strings.TrimSpace(revision) + ".json"
}

func WorkspaceLatestKey(workspaceRef string) string {
	return "workspaces/" + strings.TrimSpace(workspaceRef) + "/latest.json"
}

func WorkspaceBlobKey(workspaceRef string, sha256 string) string {
	return "workspaces/" + strings.TrimSpace(workspaceRef) + "/blobs/" + strings.TrimSpace(sha256)
}

func SessionRestoreKey(sessionRef string, revision string) string {
	return "sessions/" + strings.TrimSpace(sessionRef) + "/restore/" + strings.TrimSpace(revision) + ".json"
}
