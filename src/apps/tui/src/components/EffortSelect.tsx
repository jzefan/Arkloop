import { useKeyboard } from "@opentui/solid"
import { createSignal, For, Show } from "solid-js"
import { applyCurrentEffort, currentEffort, currentModelSupportsReasoning, setOverlay } from "../store/app"
import { effortLevels, effortSymbol, formatEffort } from "../lib/effort"
import { activeText, tuiTheme } from "../lib/theme"
import { OverlaySurface } from "./OverlaySurface"

export function EffortSelect() {
  const initialIndex = Math.max(0, effortLevels.indexOf(currentEffort()))
  const [selectedIndex, setSelectedIndex] = createSignal(initialIndex)

  useKeyboard((key) => {
    if (key.name === "escape") {
      setOverlay(null)
      return
    }

    if (key.name === "up") {
      setSelectedIndex((prev) => Math.max(0, prev - 1))
      return
    }

    if (key.name === "down") {
      setSelectedIndex((prev) => Math.min(effortLevels.length - 1, prev + 1))
      return
    }

    if (key.name === "return") {
      if (!currentModelSupportsReasoning()) {
        setOverlay(null)
        return
      }
      applyCurrentEffort(effortLevels[selectedIndex()])
      setOverlay(null)
    }
  })

  return (
    <OverlaySurface title="Effort" width={28}>
      <Show
        when={currentModelSupportsReasoning()}
        fallback={
          <box paddingLeft={2} paddingRight={2} paddingTop={1} paddingBottom={1}>
            <text content="Current model does not support effort" fg={tuiTheme.textMuted} />
          </box>
        }
      >
        <box flexDirection="column" paddingLeft={2} paddingRight={2} paddingTop={1} paddingBottom={1}>
          <For each={effortLevels}>
            {(level, index) => {
              const active = () => index() === selectedIndex()
              const current = () => level === currentEffort()
              return (
                <box
                  flexDirection="row"
                  justifyContent="space-between"
                  paddingLeft={1}
                  paddingRight={1}
                  backgroundColor={active() ? tuiTheme.primary : tuiTheme.panel}
                  onMouseDown={() => setSelectedIndex(index())}
                  onMouseUp={() => {
                    applyCurrentEffort(level)
                    setOverlay(null)
                  }}
                >
                  <box flexDirection="row" gap={1}>
                    <text content={effortSymbol(level)} fg={active() ? activeText : tuiTheme.primary} />
                    <text content={formatEffort(level)} fg={active() ? activeText : tuiTheme.text} />
                  </box>
                  <text content={current() ? "✓" : " "} fg={active() ? activeText : tuiTheme.textMuted} />
                </box>
              )
            }}
          </For>
        </box>
      </Show>
    </OverlaySurface>
  )
}
