import { Index, Show, type Accessor } from "solid-js"
import { error, historyTurns, liveAssistantTurn, pendingUserInput, setError, streaming } from "../store/chat"
import type { AssistantTurnUi, CopBlockItem } from "../lib/assistantTurn"
import { MessageBubble } from "./MessageBubble"
import { jsonStringifyForDebugDisplay } from "../lib/runTurns"
import { tuiTheme } from "../lib/theme"

export function MessageList() {
  function renderTurn(turn: typeof historyTurns[number], isLive: boolean) {
    return (
      <>
        <Show when={turn.userInput}>
          <MessageBubble role="user" content={turn.userInput ?? ""} />
        </Show>
        <Index each={turn.segments}>
          {(segment) => {
            const current = segment()
            if (current.kind === "assistant") {
              return <MessageBubble role="assistant" content={current.text} streaming={isLive && streaming()} />
            }
            if (current.kind === "tool_call") {
              return (
                <MessageBubble
                  role="tool"
                  toolName={current.toolName}
                  content={jsonStringifyForDebugDisplay(current.argsJSON)}
                />
              )
            }
            return (
              <MessageBubble
                role="tool"
                toolName={current.toolName}
                content={jsonStringifyForDebugDisplay(current.resultJSON ?? {})}
              />
            )
          }}
        </Index>
      </>
    )
  }

  function renderLiveCopItem(item: Accessor<CopBlockItem>) {
    const current = item()
    if (current.kind === "thinking") return null
    if (current.kind === "assistant_text") {
      return <MessageBubble role="assistant" content={current.content} streaming={streaming()} />
    }
    return (
      <>
        <MessageBubble
          role="tool"
          toolName={current.call.toolName}
          content={jsonStringifyForDebugDisplay(current.call.arguments)}
        />
        <Show when={current.call.result !== undefined}>
          <MessageBubble
            role="tool"
            toolName={current.call.toolName}
            content={jsonStringifyForDebugDisplay(current.call.result)}
          />
        </Show>
      </>
    )
  }

  function renderLiveAssistantTurn(turn: Accessor<AssistantTurnUi>) {
    return (
      <Index each={turn().segments}>
        {(segment) => {
          const current = segment()
          if (current.type === "text") {
            return <MessageBubble role="assistant" content={current.content} streaming={streaming()} />
          }
          return (
            <Index each={current.items}>
              {(item) => renderLiveCopItem(item)}
            </Index>
          )
        }}
      </Index>
    )
  }

  return (
    <scrollbox stickyScroll={true} stickyStart="bottom" flexGrow={1} width="100%">
      <box flexDirection="column" justifyContent="flex-end" width="100%" height="100%">
        <Index each={historyTurns}>
          {(turn) => renderTurn(turn(), false)}
        </Index>
        <Show when={pendingUserInput()}>
          <MessageBubble role="user" content={pendingUserInput() ?? ""} />
        </Show>
        <Show when={liveAssistantTurn()}>
          {(turn: Accessor<AssistantTurnUi>) => renderLiveAssistantTurn(turn)}
        </Show>
        <Show when={error()}>
          <box width="100%" paddingBottom={1} flexDirection="row" gap={1}>
            <text content="error" fg={tuiTheme.error} />
            <text content={error() ?? ""} fg={tuiTheme.textMuted} />
            <text content="dismiss" fg={tuiTheme.textMuted} onMouseUp={() => setError(null)} />
          </box>
        </Show>
      </box>
    </scrollbox>
  )
}
