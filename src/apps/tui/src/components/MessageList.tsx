import { For, Index, Show, type Accessor } from "solid-js"
import { error, historyTurns, liveAssistantTurn, pendingUserInput, setError, streaming } from "../store/chat"
import type { AssistantTurnUi, CopBlockItem } from "../lib/assistantTurn"
import { MessageBubble } from "./MessageBubble"
import { StartupCard } from "./StartupCard"
import { compressTurnSegments, summarizeLiveToolCall } from "../lib/toolSummary"
import { tuiTheme } from "../lib/theme"
import type { LlmTurn } from "../lib/runTurns"

type RenderEntry =
  | { kind: "history"; turn: LlmTurn }
  | { kind: "live"; input: string | null; turn: AssistantTurnUi | null }

export function MessageList() {
  const showStartupCard = () => (
    historyTurns.length === 0
    && pendingUserInput() === null
    && liveAssistantTurn() === null
    && !error()
  )

  function renderTurn(turn: LlmTurn, isLive: boolean) {
    const segments = compressTurnSegments(turn.segments)
    return (
      <box flexDirection="column" width="100%">
        <Show when={turn.userInput}>
          <MessageBubble role="user" content={turn.userInput ?? ""} />
        </Show>
        <Index each={segments}>
          {(segment) => {
            const current = segment()
            if (current.kind === "tool") {
              return (
                <MessageBubble
                  role="tool"
                  toolName={current.tool.toolName}
                  toolSummary={current.tool.summary}
                  toolStatus={current.tool.status}
                  toolError={current.tool.errorSummary}
                />
              )
            }
            return <MessageBubble role="assistant" content={current.text} streaming={isLive && streaming()} />
          }}
        </Index>
      </box>
    )
  }

  function renderLiveCopItem(item: Accessor<CopBlockItem>) {
    const current = item()
    if (current.kind === "thinking") return null
    if (current.kind === "assistant_text") {
      return <MessageBubble role="assistant" content={current.content} streaming={streaming()} />
    }
    const tool = summarizeLiveToolCall(current.call)
    return (
      <MessageBubble
        role="tool"
        toolName={tool.toolName}
        toolSummary={tool.summary}
        toolStatus={tool.status}
        toolError={tool.errorSummary}
      />
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

  function renderLiveTail(input: string | null, turn: AssistantTurnUi | null) {
    if (input === null && turn === null) return null
    return (
      <box flexDirection="column" width="100%">
        <Show when={input !== null}>
          <MessageBubble role="user" content={input ?? ""} />
        </Show>
        <Show when={turn}>
          {(currentTurn: Accessor<AssistantTurnUi>) => renderLiveAssistantTurn(currentTurn)}
        </Show>
      </box>
    )
  }

  function renderEntries(): RenderEntry[] {
    const entries: RenderEntry[] = historyTurns.map((turn) => ({ kind: "history", turn }))
    const input = pendingUserInput()
    const turn = liveAssistantTurn()
    if (input !== null || turn !== null) {
      entries.push({ kind: "live", input, turn })
    }
    return entries
  }

  return (
    <scrollbox stickyScroll={true} stickyStart="bottom" flexGrow={1} width="100%">
      <box flexDirection="column" width="100%" minHeight="100%">
        <Show when={showStartupCard()} fallback={<box flexGrow={1} width="100%" />}>
          <StartupCard />
        </Show>
        <For each={renderEntries()}>
          {(entry) => {
            if (entry.kind === "history") {
              return renderTurn(entry.turn, false)
            }
            return renderLiveTail(entry.input, entry.turn)
          }}
        </For>
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
