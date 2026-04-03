package napcat

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateOneBotConfig(t *testing.T) {
	cfg := GenerateOneBotConfig(6098, "test-token", "http://127.0.0.1:19001/v1/napcat/onebot-callback", "cb-token", 3000, "http-token")
	if len(cfg.Network.WebsocketServers) != 1 {
		t.Fatalf("expected 1 ws server, got %d", len(cfg.Network.WebsocketServers))
	}
	ws := cfg.Network.WebsocketServers[0]
	if ws.Port != 6098 {
		t.Errorf("expected port 6098, got %d", ws.Port)
	}
	if ws.Token != "test-token" {
		t.Errorf("expected token test-token, got %s", ws.Token)
	}
	if !ws.Enable {
		t.Error("expected ws enabled")
	}
	if len(cfg.Network.HTTPClients) != 1 {
		t.Fatalf("expected 1 http client, got %d", len(cfg.Network.HTTPClients))
	}
	hc := cfg.Network.HTTPClients[0]
	if hc.URL != "http://127.0.0.1:19001/v1/napcat/onebot-callback" {
		t.Errorf("unexpected http client url: %s", hc.URL)
	}
	if hc.Token != "cb-token" {
		t.Errorf("unexpected http client token: %s", hc.Token)
	}
	if len(cfg.Network.HTTPServers) != 1 {
		t.Fatalf("expected 1 http server, got %d", len(cfg.Network.HTTPServers))
	}
	hs := cfg.Network.HTTPServers[0]
	if hs.Port != 3000 {
		t.Errorf("expected http server port 3000, got %d", hs.Port)
	}
}

func TestWriteOneBotConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := GenerateOneBotConfig(6098, "tok", "", "", 0, "")
	if err := WriteOneBotConfig(dir, cfg); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "onebot11.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("config file not created: %v", err)
	}
}

func TestWriteWebUIConfig(t *testing.T) {
	dir := t.TempDir()
	if err := WriteWebUIConfig(dir, 6099, "my-token"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "webui.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Error("empty webui config")
	}
}

func TestFindNodeBinaryDoesNotPanic(t *testing.T) {
	_, _ = FindNodeBinary()
}
