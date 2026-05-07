package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/genai"
)

func TestGeneratedVideoFromOperationDownloadsURI(t *testing.T) {
	t.Setenv("ARKLOOP_OUTBOUND_ALLOW_LOOPBACK_HTTP", "true")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write([]byte("mp4-bytes"))
	}))
	defer server.Close()

	video, err := generatedVideoFromOperation(context.Background(), &genai.GenerateVideosOperation{
		Done: true,
		Response: &genai.GenerateVideosResponse{
			GeneratedVideos: []*genai.GeneratedVideo{{
				Video: &genai.Video{URI: server.URL + "/generated.mp4"},
			}},
		},
	}, "test-model", server.Client())
	if err != nil {
		t.Fatalf("generatedVideoFromOperation: %v", err)
	}
	if string(video.Bytes) != "mp4-bytes" || video.MimeType != "video/mp4" || video.URI == "" {
		t.Fatalf("unexpected video: %#v", video)
	}
}
