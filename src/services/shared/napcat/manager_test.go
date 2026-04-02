package napcat

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateOneBotConfig(t *testing.T) {
	cfg := GenerateOneBotConfig(6098, "test-token")
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
}

func TestWriteOneBotConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := GenerateOneBotConfig(6098, "tok")
	if err := WriteOneBotConfig(dir, "12345", cfg); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "onebot11_12345.json")
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
