package localstore

import "errors"

// Sentinel errors. These are leaked to callers via error returns from store
// methods; per-draft validation failures travel through SaveResult.Failed
// instead, with stable string error codes.
var (
	ErrInvalidKBID             = errors.New("localstore: invalid kb id")
	ErrInvalidKnowledgePointID = errors.New("localstore: invalid knowledge point id")
)
