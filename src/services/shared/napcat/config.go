package napcat

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// OneBotNetworkConfig 是 NapCat 的 OneBot11 网络配置
type OneBotNetworkConfig struct {
	HTTPServers      []json.RawMessage `json:"httpServers"`
	HTTPSseServers   []json.RawMessage `json:"httpSseServers"`
	HTTPClients      []json.RawMessage `json:"httpClients"`
	WebsocketServers []WSServerConfig  `json:"websocketServers"`
	WebsocketClients []json.RawMessage `json:"websocketClients"`
	Plugins          []json.RawMessage `json:"plugins"`
}

type WSServerConfig struct {
	Name                string `json:"name"`
	Enable              bool   `json:"enable"`
	Host                string `json:"host"`
	Port                int    `json:"port"`
	MessagePostFormat   string `json:"messagePostFormat"`
	ReportSelfMessage   bool   `json:"reportSelfMessage"`
	Token               string `json:"token"`
	EnableForcePushEvent bool   `json:"enableForcePushEvent"`
	Debug               bool   `json:"debug"`
	HeartInterval       int    `json:"heartInterval"`
}

type OneBotFullConfig struct {
	Network         OneBotNetworkConfig `json:"network"`
	MusicSignURL    string              `json:"musicSignUrl"`
	EnableLocalFile bool                `json:"enableLocalFile2Url"`
	ParseMultMsg    bool                `json:"parseMultMsg"`
}

// GenerateOneBotConfig 为 Arkloop 生成 NapCat OneBot11 配置
// wsPort 是 WS Server 监听端口，wsToken 是鉴权 token
func GenerateOneBotConfig(wsPort int, wsToken string) OneBotFullConfig {
	return OneBotFullConfig{
		Network: OneBotNetworkConfig{
			HTTPServers:    []json.RawMessage{},
			HTTPSseServers: []json.RawMessage{},
			HTTPClients:    []json.RawMessage{},
			WebsocketServers: []WSServerConfig{{
				Name:                "arkloop-ws",
				Enable:              true,
				Host:                "127.0.0.1",
				Port:                wsPort,
				MessagePostFormat:   "array",
				ReportSelfMessage:   false,
				Token:               wsToken,
				EnableForcePushEvent: true,
				Debug:               false,
				HeartInterval:       30000,
			}},
			WebsocketClients: []json.RawMessage{},
			Plugins:          []json.RawMessage{},
		},
		EnableLocalFile: true,
		ParseMultMsg:    true,
	}
}

// WriteOneBotConfig 将 OneBot 配置写入 NapCat 的 config 目录
// uin 是 QQ 号，configDir 是 NapCat 安装目录下的 config/ 路径
func WriteOneBotConfig(configDir string, uin string, cfg OneBotFullConfig) error {
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("mkdir config: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal onebot config: %w", err)
	}
	filename := fmt.Sprintf("onebot11_%s.json", uin)
	return os.WriteFile(filepath.Join(configDir, filename), data, 0644)
}

// NapCatWebUIConfig 是 NapCat WebUI 配置
type NapCatWebUIConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Token    string `json:"token"`
	LoginRate int   `json:"loginRate"`
}

// WriteWebUIConfig 写入 WebUI 配置
func WriteWebUIConfig(configDir string, port int, token string) error {
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("mkdir config: %w", err)
	}
	cfg := NapCatWebUIConfig{
		Host:      "127.0.0.1",
		Port:      port,
		Token:     token,
		LoginRate: 3,
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal webui config: %w", err)
	}
	return os.WriteFile(filepath.Join(configDir, "webui.json"), data, 0644)
}
