package data

import (
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestMapLlmRouteWriteError_sqliteUniqueModel(t *testing.T) {
	cred := uuid.MustParse("daf76e8d-f87c-4ac1-8d20-fc1ab65b0049")
	err := errors.New(`constraint failed: UNIQUE constraint failed: index 'ux_llm_routes_credential_model' (2067)`)
	got := mapLlmRouteWriteError(err, cred, "gpt-4o")
	var conflict LlmRouteModelConflictError
	if !errors.As(got, &conflict) {
		t.Fatalf("want LlmRouteModelConflictError, got %T %v", got, got)
	}
	if conflict.CredentialID != cred || conflict.Model != "gpt-4o" {
		t.Fatalf("conflict: %+v", conflict)
	}
}

func TestMapLlmRouteWriteError_sqliteNonUnique(t *testing.T) {
	cred := uuid.MustParse("daf76e8d-f87c-4ac1-8d20-fc1ab65b0049")
	err := errors.New("database is locked (5) (SQLITE_BUSY)")
	got := mapLlmRouteWriteError(err, cred, "x")
	if got != err {
		t.Fatalf("want original err, got %v", got)
	}
}
