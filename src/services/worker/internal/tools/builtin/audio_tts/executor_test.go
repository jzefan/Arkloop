package audiotts

import (
	"context"
	"testing"

	"arkloop/services/shared/objectstore"
	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/tools"

	"github.com/google/uuid"
)

type recordingStore struct {
	key         string
	data        []byte
	contentType string
}

func (s *recordingStore) PutObject(_ context.Context, key string, data []byte, options objectstore.PutOptions) error {
	s.key = key
	s.data = append([]byte(nil), data...)
	s.contentType = options.ContentType
	return nil
}

func (s *recordingStore) Put(ctx context.Context, key string, data []byte) error {
	return s.PutObject(ctx, key, data, objectstore.PutOptions{})
}

func (s *recordingStore) Get(context.Context, string) ([]byte, error) {
	return nil, context.DeadlineExceeded
}
func (s *recordingStore) GetWithContentType(context.Context, string) ([]byte, string, error) {
	return nil, "", context.DeadlineExceeded
}
func (s *recordingStore) Head(context.Context, string) (objectstore.ObjectInfo, error) {
	return objectstore.ObjectInfo{}, context.DeadlineExceeded
}
func (s *recordingStore) Delete(context.Context, string) error { return nil }
func (s *recordingStore) ListPrefix(context.Context, string) ([]objectstore.ObjectInfo, error) {
	return nil, nil
}

func TestAudioTTSExecuteStoresGeneratedSpeech(t *testing.T) {
	store := &recordingStore{}
	exec := NewToolExecutor(store, nil, nil, nil)
	exec.synthesize = func(_ context.Context, _ llm.ResolvedGatewayConfig, req SpeechRequest) (GeneratedSpeech, error) {
		if req.Text != "旁白正文" {
			t.Fatalf("unexpected text: %q", req.Text)
		}
		if req.Voice != "alloy" {
			t.Fatalf("unexpected voice: %q", req.Voice)
		}
		return GeneratedSpeech{Bytes: []byte("mp3-bytes"), MimeType: "audio/mpeg", ProviderKind: "openai", Model: "tts-test"}, nil
	}
	accountID := uuid.New()
	runID := uuid.New()

	result := exec.Execute(context.Background(), ToolName, map[string]any{
		"text":          "旁白正文",
		"voice":         "alloy",
		"artifact_name": "voice_over",
	}, tools.ExecutionContext{AccountID: &accountID, RunID: runID}, "call-1")

	if result.Error != nil {
		t.Fatalf("Execute() error = %v", result.Error)
	}
	if store.contentType != "audio/mpeg" {
		t.Fatalf("stored content type = %q", store.contentType)
	}
	if string(store.data) != "mp3-bytes" {
		t.Fatalf("stored data = %q", string(store.data))
	}
	if store.key != accountID.String()+"/"+runID.String()+"/voice_over.mp3" {
		t.Fatalf("stored key = %q", store.key)
	}
}
