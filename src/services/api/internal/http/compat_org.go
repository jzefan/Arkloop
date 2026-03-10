package http

import (
	"mime"
	nethttp "net/http"
	"path"
	"strings"
)

type apiKeyResponse struct {
	ID         string   `json:"id"`
	OrgID      string   `json:"org_id"`
	UserID     string   `json:"user_id"`
	Name       string   `json:"name"`
	KeyPrefix  string   `json:"key_prefix"`
	Scopes     []string `json:"scopes"`
	RevokedAt  *string  `json:"revoked_at,omitempty"`
	LastUsedAt *string  `json:"last_used_at,omitempty"`
	CreatedAt  string   `json:"created_at"`
}

type webhookEndpointResponse struct {
	ID        string   `json:"id"`
	OrgID     string   `json:"org_id"`
	URL       string   `json:"url"`
	Events    []string `json:"events"`
	Enabled   bool     `json:"enabled"`
	CreatedAt string   `json:"created_at"`
}

func workspaceManifestKey(workspaceRef, revision string) string {
	return "workspaces/" + workspaceRef + "/manifests/" + revision + ".json"
}

func workspaceBlobKey(workspaceRef, sha256 string) string {
	return "workspaces/" + workspaceRef + "/blobs/" + sha256
}

type workspaceManifest struct {
	Entries []workspaceManifestEntry `json:"entries,omitempty"`
}

type workspaceManifestEntry struct {
	Path    string `json:"path"`
	Type    string `json:"type"`
	SHA256  string `json:"sha256,omitempty"`
	Deleted bool   `json:"deleted,omitempty"`
}

const workspaceEntryTypeFile = "file"

func detectWorkspaceContentType(relativePath string, content []byte) string {
	if ext := strings.ToLower(path.Ext(relativePath)); ext != "" {
		if guessed := mime.TypeByExtension(ext); strings.TrimSpace(guessed) != "" {
			return guessed
		}
	}
	return nethttp.DetectContentType(content)
}
