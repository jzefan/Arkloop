package firecracker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// Client 是 Firecracker HTTP API 的轻量封装，通过 Unix domain socket 通信。
type Client struct {
	http *http.Client
}

// NewClient 创建绑定到指定 API socket 路径的客户端。
func NewClient(apiSocketPath string) *Client {
	return &Client{
		http: &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					return (&net.Dialer{}).DialContext(ctx, "unix", apiSocketPath)
				},
			},
			Timeout: 10 * time.Second,
		},
	}
}

// MachineConfig 对应 Firecracker PUT /machine-config。
type MachineConfig struct {
	VcpuCount  int64 `json:"vcpu_count"`
	MemSizeMib int64 `json:"mem_size_mib"`
	Smt        bool  `json:"smt"`
}

// BootSource 对应 Firecracker PUT /boot-source。
type BootSource struct {
	KernelImagePath string `json:"kernel_image_path"`
	BootArgs        string `json:"boot_args"`
}

// Drive 对应 Firecracker PUT /drives/{drive_id}。
type Drive struct {
	DriveID      string `json:"drive_id"`
	PathOnHost   string `json:"path_on_host"`
	IsRootDevice bool   `json:"is_root_device"`
	IsReadOnly   bool   `json:"is_read_only"`
}

// VsockDevice 对应 Firecracker PUT /vsock。
type VsockDevice struct {
	GuestCID uint32 `json:"guest_cid"`
	UDSPath  string `json:"uds_path"`
}

// Configure 依次设置 machine-config、boot-source、rootfs drive、vsock 设备。
func (c *Client) Configure(ctx context.Context, mc MachineConfig, bs BootSource, drv Drive, vsock VsockDevice) error {
	if err := c.put(ctx, "/machine-config", mc); err != nil {
		return fmt.Errorf("machine-config: %w", err)
	}
	if err := c.put(ctx, "/boot-source", bs); err != nil {
		return fmt.Errorf("boot-source: %w", err)
	}
	if err := c.put(ctx, "/drives/"+drv.DriveID, drv); err != nil {
		return fmt.Errorf("drives/%s: %w", drv.DriveID, err)
	}
	if err := c.put(ctx, "/vsock", vsock); err != nil {
		return fmt.Errorf("vsock: %w", err)
	}
	return nil
}

// Start 向 Firecracker 发送 InstanceStart 动作，启动 microVM。
func (c *Client) Start(ctx context.Context) error {
	body := map[string]string{"action_type": "InstanceStart"}
	if err := c.put(ctx, "/actions", body); err != nil {
		return fmt.Errorf("InstanceStart: %w", err)
	}
	return nil
}

func (c *Client) put(ctx context.Context, path string, body any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, "http://localhost"+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, raw)
	}
	return nil
}
