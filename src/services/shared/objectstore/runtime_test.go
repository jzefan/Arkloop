package objectstore

import "testing"

func TestNormalizeRuntimeConfigPrefersFilesystemWhenRootPresent(t *testing.T) {
	cfg, err := NormalizeRuntimeConfig(RuntimeConfig{
		RootDir: "/tmp/arkloop-storage",
		S3Config: S3Config{
			Endpoint:  "http://seaweedfs:8333",
			AccessKey: "key",
			SecretKey: "secret",
		},
	})
	if err != nil {
		t.Fatalf("normalize runtime config: %v", err)
	}
	if cfg.Backend != BackendFilesystem {
		t.Fatalf("unexpected backend: %s", cfg.Backend)
	}
}

func TestNormalizeRuntimeConfigHonorsExplicitBackend(t *testing.T) {
	cfg, err := NormalizeRuntimeConfig(RuntimeConfig{
		Backend: BackendS3,
		RootDir: "/tmp/arkloop-storage",
		S3Config: S3Config{
			Endpoint:  "http://seaweedfs:8333",
			AccessKey: "key",
			SecretKey: "secret",
		},
	})
	if err != nil {
		t.Fatalf("normalize runtime config: %v", err)
	}
	if cfg.Backend != BackendS3 {
		t.Fatalf("unexpected backend: %s", cfg.Backend)
	}
}
