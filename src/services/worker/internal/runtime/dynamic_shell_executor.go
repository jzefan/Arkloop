//go:build desktop

package runtime

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"arkloop/services/shared/desktop"
	"arkloop/services/worker/internal/tools"
	"arkloop/services/worker/internal/tools/localshell"
	"arkloop/services/worker/internal/tools/sandboxshell"
)

type DynamicShellExecutor struct {
	mu            sync.RWMutex
	local         *localshell.Executor
	vm            *sandboxshell.Executor
	vmAddr        string
	vmToken       string
	processOwners map[string]string
}

func NewDynamicShellExecutor(vmAddr, vmToken string) *DynamicShellExecutor {
	return &DynamicShellExecutor{
		vmAddr:        strings.TrimSpace(vmAddr),
		vmToken:       vmToken,
		processOwners: map[string]string{},
	}
}

func (e *DynamicShellExecutor) ensureLocal() *localshell.Executor {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.local == nil {
		e.local = localshell.NewExecutor()
	}
	return e.local
}

func (e *DynamicShellExecutor) ensureVM() *sandboxshell.Executor {
	e.mu.Lock()
	defer e.mu.Unlock()
	addr := strings.TrimSpace(desktop.GetSandboxAddr())
	if addr != "" && addr != e.vmAddr {
		e.vmAddr = addr
		e.vm = nil
	}
	if e.vm == nil {
		e.vm = sandboxshell.NewExecutor("http://"+e.vmAddr, e.vmToken)
	}
	return e.vm
}

func (e *DynamicShellExecutor) Execute(
	ctx context.Context,
	toolName string,
	args map[string]any,
	execCtx tools.ExecutionContext,
	toolCallID string,
) tools.ExecutionResult {
	mode := strings.TrimSpace(desktop.GetExecutionMode())
	vmAddr := strings.TrimSpace(desktop.GetSandboxAddr())
	slog.Info("dynamic_shell_executor: Execute",
		"tool", toolName,
		"mode", mode,
		"vm_addr", vmAddr,
		"run_id", execCtx.RunID.String(),
	)

	backend, routeErr := e.resolveBackend(toolName, args, mode, vmAddr)
	if routeErr != nil {
		return tools.ExecutionResult{
			Error:      &tools.ExecutionError{ErrorClass: "tool.args_invalid", Message: routeErr.Error()},
			DurationMs: 0,
		}
	}

	var result tools.ExecutionResult
	switch backend {
	case "vm":
		result = e.ensureVM().Execute(ctx, toolName, args, execCtx, toolCallID)
	default:
		result = e.ensureLocal().Execute(ctx, toolName, args, execCtx, toolCallID)
	}

	e.reconcileProcessOwner(toolName, backend, args, result)
	return result
}

func (e *DynamicShellExecutor) resolveBackend(toolName string, args map[string]any, mode string, vmAddr string) (string, error) {
	switch toolName {
	case localshell.ExecCommandAgentSpec.Name:
		if mode == "vm" && vmAddr != "" {
			return "vm", nil
		}
		return "local", nil
	case localshell.ContinueProcessAgentSpec.Name,
		localshell.TerminateProcessAgentSpec.Name,
		localshell.ResizeProcessAgentSpec.Name:
		processRef, _ := args["process_ref"].(string)
		processRef = strings.TrimSpace(processRef)
		if processRef == "" {
			return "", fmt.Errorf("parameter process_ref is required")
		}
		e.mu.RLock()
		owner := e.processOwners[processRef]
		e.mu.RUnlock()
		if owner != "" {
			return owner, nil
		}
		if mode == "vm" && vmAddr != "" {
			return "vm", nil
		}
		return "local", nil
	default:
		if mode == "vm" && vmAddr != "" {
			return "vm", nil
		}
		return "local", nil
	}
}

func (e *DynamicShellExecutor) reconcileProcessOwner(toolName string, backend string, args map[string]any, result tools.ExecutionResult) {
	if result.Error != nil {
		return
	}

	switch toolName {
	case localshell.ExecCommandAgentSpec.Name:
		processRef, _ := result.ResultJSON["process_ref"].(string)
		processRef = strings.TrimSpace(processRef)
		if processRef == "" {
			return
		}
		e.mu.Lock()
		e.processOwners[processRef] = backend
		e.mu.Unlock()
	case localshell.ContinueProcessAgentSpec.Name:
		processRef, _ := args["process_ref"].(string)
		processRef = strings.TrimSpace(processRef)
		if processRef == "" {
			return
		}
		running, _ := result.ResultJSON["running"].(bool)
		if running {
			return
		}
		e.mu.Lock()
		delete(e.processOwners, processRef)
		e.mu.Unlock()
	case localshell.TerminateProcessAgentSpec.Name:
		processRef, _ := args["process_ref"].(string)
		processRef = strings.TrimSpace(processRef)
		if processRef == "" {
			return
		}
		e.mu.Lock()
		delete(e.processOwners, processRef)
		e.mu.Unlock()
	}
}
