import { Show } from "solid-js"
import { useKeyboard } from "@opentui/solid"
import { pendingToolCall, setPendingToolCall } from "../store/chat"
import { setOverlay } from "../store/app"
import type { ApiClient } from "../api/client"
import { OverlaySurface } from "./OverlaySurface"
import { tuiTheme } from "../lib/theme"

interface Props {
  client: ApiClient
}

export function ToolConfirm(props: Props) {
  useKeyboard((key) => {
    const tool = pendingToolCall()
    if (!tool) return

    if (key.name === "y") {
      respond(props.client, tool.runId, tool.toolCallId, "allow")
    } else if (key.name === "n") {
      respond(props.client, tool.runId, tool.toolCallId, "deny")
    } else if (key.name === "a") {
      respond(props.client, tool.runId, tool.toolCallId, "allow_session")
    } else if (key.name === "escape") {
      respond(props.client, tool.runId, tool.toolCallId, "deny")
    }
  })

  const argsText = () => {
    const tool = pendingToolCall()
    if (!tool) return ""
    try {
      return JSON.stringify(tool.args, null, 2)
    } catch {
      return String(tool.args)
    }
  }

  return (
    <Show when={pendingToolCall()}>
      <OverlaySurface title="Tool Confirmation" width={92}>
        <box flexDirection="column" paddingLeft={3} paddingRight={3} paddingTop={2} paddingBottom={2} gap={1}>
          <text content={pendingToolCall()?.toolName ?? "unknown"} fg={tuiTheme.warning} />
          <text content="arguments" fg={tuiTheme.textMuted} />
          <box backgroundColor={tuiTheme.element} paddingLeft={2} paddingRight={2} paddingTop={1} paddingBottom={1}>
            <text content={argsText()} wrapMode="word" fg={tuiTheme.text} />
          </box>
        </box>
        <box
          flexDirection="row"
          justifyContent="space-between"
          paddingLeft={3}
          paddingRight={3}
          paddingTop={1}
          paddingBottom={1}
          border={["top"]}
          borderColor={tuiTheme.borderSubtle}
        >
          <text content="y allow · n deny" fg={tuiTheme.textMuted} />
          <text content="a allow all" fg={tuiTheme.textMuted} />
        </box>
      </OverlaySurface>
    </Show>
  )
}

async function respond(client: ApiClient, runId: string, toolCallId: string, action: string) {
  try {
    await client.respondTool(runId, toolCallId, action)
  } catch {
    // stream will handle timeout
  }
  setPendingToolCall(null)
  setOverlay(null)
}
