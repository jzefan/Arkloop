package questionstore

import "errors"

var (
	ErrIntegrationDisabled = errors.New("questionstore: exam integration is disabled in this deployment")
	ErrUnsupportedMode     = errors.New("questionstore: unsupported integration_mode")
	ErrNotImplemented      = errors.New("questionstore: implementation not registered")
)

// NewLocalStoreFunc is set by the localstore sub-package at init time.
var NewLocalStoreFunc func(kbID string) QuestionStore

// NewExamStoreFunc is set by the examstore sub-package at init time.
var NewExamStoreFunc func(examScopeID string) QuestionStore

// For returns the appropriate QuestionStore for the given KB descriptor.
// examEnabled should reflect ARKLOOP_EXAM_INTEGRATION_ENABLED.
func For(kb KBDescriptor, examEnabled bool) (QuestionStore, error) {
	switch kb.IntegrationMode {
	case "standalone":
		if NewLocalStoreFunc == nil {
			return nil, ErrNotImplemented
		}
		return NewLocalStoreFunc(kb.ID), nil
	case "exam":
		if !examEnabled {
			return nil, ErrIntegrationDisabled
		}
		if NewExamStoreFunc == nil {
			return nil, ErrNotImplemented
		}
		return NewExamStoreFunc(kb.ExamScopeID), nil
	default:
		return nil, ErrUnsupportedMode
	}
}
