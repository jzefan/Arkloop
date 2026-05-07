package catalogapi

import (
	"errors"
	"strings"

	sharedoutbound "arkloop/services/shared/outboundurl"
)

func normalizeOptionalBaseURL(raw *string) (*string, error) {
	if raw == nil {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*raw)
	if trimmed == "" {
		return nil, nil
	}
	normalized, err := sharedoutbound.DefaultPolicy().NormalizeBaseURL(trimmed)
	if err != nil {
		return nil, err
	}
	return &normalized, nil
}

func normalizeOptionalInternalBaseURL(raw *string) (*string, error) {
	if raw == nil {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*raw)
	if trimmed == "" {
		return nil, nil
	}
	normalized, err := sharedoutbound.DefaultPolicy().NormalizeInternalBaseURL(trimmed)
	if err != nil {
		return nil, err
	}
	return &normalized, nil
}

var _ error = (*deniedURLError)(nil)

type deniedURLError struct {
	reason  string
	details map[string]any
}

func (e *deniedURLError) Error() string { return "url denied: " + e.reason }

func (e *deniedURLError) Reason() string { return e.reason }

func (e *deniedURLError) Details() map[string]any { return e.details }

func wrapDeniedError(err error) error {
	if err == nil {
		return nil
	}
	var denied sharedoutbound.DeniedError
	if !errors.As(err, &denied) {
		return err
	}
	return &deniedURLError{
		reason:  denied.Reason,
		details: denied.Details,
	}
}
