package security

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	sharedconfig "arkloop/services/shared/config"
)

type RuntimeSemanticScanner struct {
	resolver sharedconfig.Resolver
	modelDir string
	ortLib   string

	newLocal  func(SemanticScannerConfig) (SemanticClassifier, error)
	newRemote func(RemoteSemanticScannerConfig) (SemanticClassifier, error)

	mu          sync.Mutex
	current     SemanticClassifier
	provider    string
	fingerprint string
}

func NewRuntimeSemanticScanner(resolver sharedconfig.Resolver, modelDir, ortLib string) *RuntimeSemanticScanner {
	return &RuntimeSemanticScanner{
		resolver: resolver,
		modelDir: strings.TrimSpace(modelDir),
		ortLib:   strings.TrimSpace(ortLib),
		newLocal: func(cfg SemanticScannerConfig) (SemanticClassifier, error) {
			return NewSemanticScanner(cfg)
		},
		newRemote: func(cfg RemoteSemanticScannerConfig) (SemanticClassifier, error) {
			return NewRemoteSemanticScanner(cfg)
		},
	}
}

func (s *RuntimeSemanticScanner) Classify(ctx context.Context, text string) (SemanticResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	scanner, err := s.ensureScannerLocked()
	if err != nil {
		return SemanticResult{}, err
	}
	return scanner.Classify(ctx, text)
}

func (s *RuntimeSemanticScanner) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.current != nil {
		s.current.Close()
		s.current = nil
	}
	s.provider = ""
	s.fingerprint = ""
}

func (s *RuntimeSemanticScanner) ensureScannerLocked() (SemanticClassifier, error) {
	provider := strings.ToLower(runtimeResolveString(s.resolver, "security.semantic_scanner.provider", "local"))

	switch provider {
	case "local":
		if s.modelDir == "" {
			return nil, fmt.Errorf("local semantic scanner model dir not configured")
		}
		fingerprint := fmt.Sprintf("local|%s|%s", s.modelDir, s.ortLib)
		if s.current != nil && s.provider == provider && s.fingerprint == fingerprint {
			return s.current, nil
		}

		next, err := s.newLocal(SemanticScannerConfig{
			ModelDir:   s.modelDir,
			OrtLibPath: s.ortLib,
		})
		if err != nil {
			return nil, err
		}
		s.swapScannerLocked(provider, fingerprint, next)
		slog.Info("semantic scanner initialized", "provider", provider, "model_dir", s.modelDir)
		return s.current, nil

	case "api":
		baseURL := runtimeResolveString(s.resolver, "security.semantic_scanner.api_endpoint", "")
		apiKey := runtimeResolveString(s.resolver, "security.semantic_scanner.api_key", "")
		model := runtimeResolveString(s.resolver, "security.semantic_scanner.api_model", defaultRemoteSemanticModel)
		timeoutMs := runtimeResolveInt(s.resolver, "security.semantic_scanner.api_timeout_ms", 4000)
		fingerprint := fmt.Sprintf("api|%s|%s|%s|%d", baseURL, apiKey, model, timeoutMs)
		if s.current != nil && s.provider == provider && s.fingerprint == fingerprint {
			return s.current, nil
		}

		next, err := s.newRemote(RemoteSemanticScannerConfig{
			BaseURL: baseURL,
			APIKey:  apiKey,
			Model:   model,
			Timeout: time.Duration(timeoutMs) * time.Millisecond,
		})
		if err != nil {
			return nil, err
		}
		s.swapScannerLocked(provider, fingerprint, next)
		slog.Info("semantic scanner initialized", "provider", provider, "base_url", baseURL, "model", model)
		return s.current, nil

	default:
		return nil, fmt.Errorf("semantic scanner provider unsupported: %s", provider)
	}
}

func (s *RuntimeSemanticScanner) swapScannerLocked(provider, fingerprint string, next SemanticClassifier) {
	if s.current != nil {
		s.current.Close()
	}
	s.current = next
	s.provider = provider
	s.fingerprint = fingerprint
}

func runtimeResolveString(resolver sharedconfig.Resolver, key, fallback string) string {
	if resolver == nil {
		return fallback
	}
	val, err := resolver.Resolve(context.Background(), key, sharedconfig.Scope{})
	if err != nil || strings.TrimSpace(val) == "" {
		return fallback
	}
	return strings.TrimSpace(val)
}

func runtimeResolveInt(resolver sharedconfig.Resolver, key string, fallback int) int {
	raw := runtimeResolveString(resolver, key, "")
	if raw == "" {
		return fallback
	}
	var value int
	if _, err := fmt.Sscanf(strings.TrimSpace(raw), "%d", &value); err != nil {
		return fallback
	}
	return value
}
