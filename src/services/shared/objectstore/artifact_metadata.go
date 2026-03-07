package objectstore

import "strings"

const (
	ArtifactOwnerKindRun = "run"

	ArtifactMetaOwnerKind = "owner_kind"
	ArtifactMetaOwnerID   = "owner_id"
	ArtifactMetaOrgID     = "org_id"
	ArtifactMetaThreadID  = "thread_id"
)

func ArtifactMetadata(ownerKind, ownerID, orgID string, threadID *string) map[string]string {
	metadata := map[string]string{}
	putArtifactMetadata(metadata, ArtifactMetaOwnerKind, ownerKind)
	putArtifactMetadata(metadata, ArtifactMetaOwnerID, ownerID)
	putArtifactMetadata(metadata, ArtifactMetaOrgID, orgID)
	if threadID != nil {
		putArtifactMetadata(metadata, ArtifactMetaThreadID, *threadID)
	}
	if len(metadata) == 0 {
		return nil
	}
	return metadata
}

func putArtifactMetadata(target map[string]string, key string, value string) {
	cleaned := strings.TrimSpace(value)
	if cleaned == "" {
		return
	}
	target[key] = cleaned
}
