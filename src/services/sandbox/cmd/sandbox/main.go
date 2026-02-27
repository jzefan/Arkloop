package main

import (
"context"
"os"

"arkloop/services/sandbox/internal/app"
sandboxhttp "arkloop/services/sandbox/internal/http"
"arkloop/services/sandbox/internal/logging"
"arkloop/services/sandbox/internal/session"
)

func main() {
if err := run(); err != nil {
_, _ = os.Stderr.WriteString(err.Error() + "\n")
os.Exit(1)
}
}

func run() error {
if _, err := app.LoadDotenvIfEnabled(false); err != nil {
return err
}

cfg, err := app.LoadConfigFromEnv()
if err != nil {
return err
}

logger := logging.NewJSONLogger("sandbox", os.Stdout)

mgr := session.NewManager(session.ManagerConfig{
FirecrackerBin:     cfg.FirecrackerBin,
KernelImagePath:    cfg.KernelImagePath,
RootfsPath:         cfg.RootfsPath,
SocketBaseDir:      cfg.SocketBaseDir,
BootTimeoutSeconds: cfg.BootTimeoutSeconds,
GuestAgentPort:     cfg.GuestAgentPort,
MaxSessions:        cfg.MaxSessions,
})

handler := sandboxhttp.NewHandler(mgr, logger)

application, err := app.NewApplication(cfg, logger, mgr)
if err != nil {
return err
}
return application.Run(context.Background(), handler)
}
