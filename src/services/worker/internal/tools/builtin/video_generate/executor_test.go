package videogenerate

import (
	"context"
	"testing"

	sharedconfig "arkloop/services/shared/config"
	"arkloop/services/shared/messagecontent"
	"arkloop/services/shared/objectstore"
	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/routing"
	"arkloop/services/worker/internal/tools"

	"github.com/google/uuid"
)

type testStore struct {
	key     string
	data    []byte
	options objectstore.PutOptions
}

func (s *testStore) PutObject(_ context.Context, key string, data []byte, options objectstore.PutOptions) error {
	s.key = key
	s.data = append([]byte(nil), data...)
	s.options = options
	return nil
}

func (s *testStore) Put(_ context.Context, key string, data []byte) error {
	return s.PutObject(context.Background(), key, data, objectstore.PutOptions{})
}

func (s *testStore) Get(_ context.Context, _ string) ([]byte, error) {
	return nil, context.DeadlineExceeded
}

func (s *testStore) GetWithContentType(_ context.Context, _ string) ([]byte, string, error) {
	return nil, "", context.DeadlineExceeded
}

func (s *testStore) Head(_ context.Context, _ string) (objectstore.ObjectInfo, error) {
	return objectstore.ObjectInfo{}, context.DeadlineExceeded
}

func (s *testStore) Delete(_ context.Context, _ string) error { return nil }

func (s *testStore) ListPrefix(_ context.Context, _ string) ([]objectstore.ObjectInfo, error) {
	return nil, nil
}

type resolverStub struct {
	value string
}

func (r resolverStub) Resolve(_ context.Context, _ string, _ sharedconfig.Scope) (string, error) {
	return r.value, nil
}

func (r resolverStub) ResolvePrefix(_ context.Context, _ string, _ sharedconfig.Scope) (map[string]string, error) {
	return map[string]string{}, nil
}

type referenceImageRunContext struct {
	messages []llm.Message
}

func (r referenceImageRunContext) ReadToolMessages() []llm.Message {
	return r.messages
}

func TestToolExecutorExecuteWritesVideoArtifact(t *testing.T) {
	accountID := uuid.New()
	routingLoader := routing.NewConfigLoader(nil, routing.ProviderRoutingConfig{
		Credentials: []routing.ProviderCredential{{
			ID:           "cred-zenmux",
			Name:         "zenmux",
			OwnerKind:    routing.CredentialScopePlatform,
			ProviderKind: routing.ProviderKindGemini,
			APIKeyValue:  stringPtr("sk-ss-v1-test"),
			BaseURL:      stringPtr("https://zenmux.ai/api/vertex-ai"),
		}},
		Routes: []routing.ProviderRouteRule{{
			ID:           "route-video",
			Model:        "google/veo-3.1-generate-001",
			CredentialID: "cred-zenmux",
		}},
		DefaultRouteID: "route-video",
	})
	store := &testStore{}
	executor := NewToolExecutor(store, nil, resolverStub{value: "zenmux^google/veo-3.1-generate-001"}, routingLoader)
	executor.generate = func(_ context.Context, cfg llm.ResolvedGatewayConfig, req llm.VideoGenerationRequest) (llm.GeneratedVideo, error) {
		if cfg.Model != "google/veo-3.1-generate-001" {
			t.Fatalf("unexpected model: %s", cfg.Model)
		}
		if req.Prompt != "cinematic product shot" || req.AspectRatio != "16:9" || req.DurationSeconds != 8 {
			t.Fatalf("unexpected request: %#v", req)
		}
		return llm.GeneratedVideo{
			Bytes:        []byte("mp4-bytes"),
			MimeType:     "video/mp4",
			ProviderKind: "zenmux",
			Model:        cfg.Model,
		}, nil
	}

	runID := uuid.New()
	threadID := uuid.New()
	result := executor.Execute(context.Background(), AgentSpec.Name, map[string]any{
		"prompt":           "cinematic product shot",
		"aspect_ratio":     "16:9",
		"duration_seconds": float64(8),
	}, tools.ExecutionContext{
		RunID:     runID,
		AccountID: &accountID,
		ThreadID:  &threadID,
	}, "call_1")
	if result.Error != nil {
		t.Fatalf("unexpected error: %#v", result.Error)
	}
	if store.key != accountID.String()+"/"+runID.String()+"/generated-video.mp4" {
		t.Fatalf("unexpected artifact key: %s", store.key)
	}
	if store.options.ContentType != "video/mp4" {
		t.Fatalf("unexpected content type: %s", store.options.ContentType)
	}
	artifacts, ok := result.ResultJSON["artifacts"].([]map[string]any)
	if !ok || len(artifacts) != 1 || artifacts[0]["display"] != "inline" {
		t.Fatalf("unexpected artifacts: %#v", result.ResultJSON["artifacts"])
	}
}

func TestToolExecutorUsesLatestUserImageAsInputImage(t *testing.T) {
	accountID := uuid.New()
	routingLoader := routing.NewConfigLoader(nil, routing.ProviderRoutingConfig{
		Credentials: []routing.ProviderCredential{{
			ID:           "cred-zenmux",
			Name:         "zenmux",
			OwnerKind:    routing.CredentialScopePlatform,
			ProviderKind: routing.ProviderKindGemini,
			APIKeyValue:  stringPtr("sk-ss-v1-test"),
			BaseURL:      stringPtr("https://zenmux.ai/api/vertex-ai"),
		}},
		Routes: []routing.ProviderRouteRule{{
			ID:           "route-video",
			Model:        "google/veo-3.1-generate-001",
			CredentialID: "cred-zenmux",
		}},
		DefaultRouteID: "route-video",
	})
	store := &testStore{}
	executor := NewToolExecutor(store, nil, resolverStub{value: "zenmux^google/veo-3.1-generate-001"}, routingLoader)
	executor.generate = func(_ context.Context, _ llm.ResolvedGatewayConfig, req llm.VideoGenerationRequest) (llm.GeneratedVideo, error) {
		if req.InputImage == nil || string(req.InputImage.Data) != "image-bytes" {
			t.Fatalf("expected reference image in request, got %#v", req.InputImage)
		}
		return llm.GeneratedVideo{
			Bytes:        []byte("mp4-bytes"),
			MimeType:     "video/mp4",
			ProviderKind: "zenmux",
			Model:        "google/veo-3.1-generate-001",
		}, nil
	}

	result := executor.Execute(context.Background(), AgentSpec.Name, map[string]any{
		"prompt": "make the dog dance",
	}, tools.ExecutionContext{
		RunID:     uuid.New(),
		AccountID: &accountID,
		PipelineRC: referenceImageRunContext{messages: []llm.Message{{
			Role: "user",
			Content: []llm.ContentPart{{
				Type:       messagecontent.PartTypeImage,
				Attachment: &messagecontent.AttachmentRef{Key: "msg/image.png", MimeType: "image/png"},
				Data:       []byte("image-bytes"),
			}},
		}}},
	}, "call_1")
	if result.Error != nil {
		t.Fatalf("unexpected error: %#v", result.Error)
	}
}

func TestToolExecutorExecuteCoercesZenMuxOpenAIVideoModelToVertex(t *testing.T) {
	accountID := uuid.New()
	routingLoader := routing.NewConfigLoader(nil, routing.ProviderRoutingConfig{
		Credentials: []routing.ProviderCredential{{
			ID:           "cred-zenmux-openai",
			Name:         "OpenAI (Chat Completions)",
			OwnerKind:    routing.CredentialScopePlatform,
			ProviderKind: routing.ProviderKindOpenAI,
			APIKeyValue:  stringPtr("sk-ss-v1-test"),
			BaseURL:      stringPtr("https://zenmux.ai/api/v1"),
			OpenAIMode:   stringPtr("chat_completions"),
		}},
		Routes: []routing.ProviderRouteRule{{
			ID:           "route-chat",
			Model:        "anthropic/claude-sonnet-4.6",
			CredentialID: "cred-zenmux-openai",
		}},
		DefaultRouteID: "route-chat",
	})
	store := &testStore{}
	executor := NewToolExecutor(store, nil, resolverStub{value: "OpenAI (Chat Completions)^google/veo-3.1-generate-001"}, routingLoader)
	executor.generate = func(_ context.Context, cfg llm.ResolvedGatewayConfig, _ llm.VideoGenerationRequest) (llm.GeneratedVideo, error) {
		if cfg.ProtocolKind != llm.ProtocolKindGeminiGenerateContent {
			t.Fatalf("expected gemini protocol, got %s", cfg.ProtocolKind)
		}
		if cfg.Transport.BaseURL != routing.ZenMuxVertexAIBaseURL {
			t.Fatalf("unexpected base url: %s", cfg.Transport.BaseURL)
		}
		if cfg.Model != "google/veo-3.1-generate-001" {
			t.Fatalf("unexpected model: %s", cfg.Model)
		}
		return llm.GeneratedVideo{
			Bytes:        []byte("mp4-bytes"),
			MimeType:     "video/mp4",
			ProviderKind: "zenmux",
			Model:        cfg.Model,
		}, nil
	}

	result := executor.Execute(context.Background(), AgentSpec.Name, map[string]any{
		"prompt": "cinematic product shot",
	}, tools.ExecutionContext{
		RunID:     uuid.New(),
		AccountID: &accountID,
	}, "call_1")
	if result.Error != nil {
		t.Fatalf("unexpected error: %#v", result.Error)
	}
}

func stringPtr(s string) *string { return &s }
