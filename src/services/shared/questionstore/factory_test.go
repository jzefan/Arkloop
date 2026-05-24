package questionstore

import "testing"

func TestFor_ExamMode_DisabledReturnsErr(t *testing.T) {
	_, err := For(KBDescriptor{IntegrationMode: "exam", ExamScopeID: "s1"}, false)
	if err != ErrIntegrationDisabled {
		t.Errorf("want ErrIntegrationDisabled, got %v", err)
	}
}

func TestFor_UnknownMode_ReturnsErr(t *testing.T) {
	_, err := For(KBDescriptor{IntegrationMode: "weird"}, true)
	if err != ErrUnsupportedMode {
		t.Errorf("want ErrUnsupportedMode, got %v", err)
	}
}

func TestFor_Standalone_CallsNewLocalStoreFunc(t *testing.T) {
	called := false
	NewLocalStoreFunc = func(kbID string) QuestionStore {
		called = true
		return nil
	}
	defer func() { NewLocalStoreFunc = nil }()

	_, _ = For(KBDescriptor{IntegrationMode: "standalone", ID: "kb1"}, false)
	if !called {
		t.Error("NewLocalStoreFunc not called")
	}
}

func TestFor_Exam_CallsNewExamStoreFunc(t *testing.T) {
	called := false
	NewExamStoreFunc = func(examScopeID string) QuestionStore {
		called = true
		if examScopeID != "s1" {
			t.Errorf("want examScopeID=s1, got %s", examScopeID)
		}
		return nil
	}
	defer func() { NewExamStoreFunc = nil }()

	_, _ = For(KBDescriptor{IntegrationMode: "exam", ExamScopeID: "s1"}, true)
	if !called {
		t.Error("NewExamStoreFunc not called")
	}
}

func TestFor_Standalone_NilFunc_ReturnsNotImplemented(t *testing.T) {
	NewLocalStoreFunc = nil
	_, err := For(KBDescriptor{IntegrationMode: "standalone"}, false)
	if err != ErrNotImplemented {
		t.Errorf("want ErrNotImplemented, got %v", err)
	}
}
