import type { InputRenderable, ScrollBoxRenderable } from "@opentui/core"
import { useKeyboard } from "@opentui/solid"
import { createEffect, createMemo, createSignal, For, on, Show } from "solid-js"
import { activeText, tuiTheme } from "../lib/theme"
import { OverlaySurface } from "./OverlaySurface"

export interface PickerItem {
  value: string
  title: string
  description?: string
  meta?: string
}

interface Props {
  title: string
  items: PickerItem[]
  currentValue?: string | null
  loading?: boolean
  emptyText?: string
  placeholder?: string
  onClose: () => void
  onSelect: (item: PickerItem) => void
}

export function PickerOverlay(props: Props) {
  const [query, setQuery] = createSignal("")
  const [selectedIndex, setSelectedIndex] = createSignal(0)
  let scroll: ScrollBoxRenderable | undefined
  let input: InputRenderable | undefined

  const filteredItems = createMemo(() => {
    const needle = query().trim().toLowerCase()
    if (!needle) return props.items
    return props.items.filter((item) => {
      const haystacks = [item.title, item.description ?? "", item.meta ?? ""]
      return haystacks.some((value) => value.toLowerCase().includes(needle))
    })
  })

  createEffect(on(
    [filteredItems, query, () => props.currentValue],
    ([items, currentQuery, currentValue]) => {
      if (items.length === 0) {
        setSelectedIndex(0)
        return
      }
      const currentIndex = currentValue ? items.findIndex((item) => item.value === currentValue) : -1
      if (currentIndex >= 0 && currentQuery.trim() === "") {
        setSelectedIndex(currentIndex)
        return
      }
      if (selectedIndex() >= items.length) {
        setSelectedIndex(items.length - 1)
      }
    },
  ))

  createEffect(() => {
    const selected = filteredItems()[selectedIndex()]
    if (!selected || !scroll) return
    const row = scroll.getChildren().find((child) => child.id === selected.value)
    if (!row) return
    const relativeTop = row.y - scroll.y
    if (relativeTop < 0) {
      scroll.scrollBy(relativeTop)
      return
    }
    if (relativeTop >= scroll.height) {
      scroll.scrollBy(relativeTop - scroll.height + 1)
    }
  })

  createEffect(() => {
    if (!input || input.isDestroyed) return
    setTimeout(() => {
      if (!input || input.isDestroyed || input.focused) return
      input.focus()
    }, 0)
  })

  useKeyboard((key) => {
    if (key.name === "escape") {
      props.onClose()
      return
    }

    if (key.name === "up") {
      move(-1)
      return
    }

    if (key.name === "down") {
      move(1)
      return
    }

    if (key.name === "pageup") {
      move(-8)
      return
    }

    if (key.name === "pagedown") {
      move(8)
      return
    }

    if (key.name === "return") {
      const item = filteredItems()[selectedIndex()]
      if (item) props.onSelect(item)
    }
  })

  function move(delta: number) {
    const items = filteredItems()
    if (items.length === 0) return
    let next = selectedIndex() + delta
    if (next < 0) next = 0
    if (next > items.length - 1) next = items.length - 1
    setSelectedIndex(next)
  }

  return (
    <OverlaySurface title={props.title}>
      <box paddingLeft={3} paddingRight={3} paddingTop={1} paddingBottom={1} backgroundColor={tuiTheme.panel}>
        <input
          ref={(item: InputRenderable) => {
            input = item
          }}
          placeholder={props.placeholder ?? "Search"}
          placeholderColor={tuiTheme.textMuted}
          textColor={tuiTheme.text}
          focusedTextColor={tuiTheme.text}
          focusedBackgroundColor={tuiTheme.element}
          cursorColor={tuiTheme.primary}
          onInput={(value) => setQuery(value)}
        />
      </box>
      <box paddingLeft={1} paddingRight={1} paddingBottom={1}>
        <Show
          when={!props.loading && filteredItems().length > 0}
          fallback={
            <box paddingLeft={2} paddingRight={2} paddingTop={1} paddingBottom={1}>
              <text
                content={props.loading ? "Loading..." : (props.emptyText ?? "No results")}
                fg={tuiTheme.textMuted}
              />
            </box>
          }
        >
          <scrollbox ref={(item: ScrollBoxRenderable) => { scroll = item }} maxHeight={16} scrollbarOptions={{ visible: false }}>
            <For each={filteredItems()}>
              {(item, index) => {
                const active = () => index() === selectedIndex()
                const current = () => item.value === props.currentValue
                return (
                  <box
                    id={item.value}
                    flexDirection="row"
                    justifyContent="space-between"
                    paddingLeft={2}
                    paddingRight={2}
                    backgroundColor={active() ? tuiTheme.primary : tuiTheme.panel}
                    onMouseDown={() => setSelectedIndex(index())}
                    onMouseUp={() => props.onSelect(item)}
                  >
                    <box flexDirection="row" gap={1} flexGrow={1}>
                      <Show when={current()}>
                        <text content="●" fg={active() ? activeText : tuiTheme.primary} />
                      </Show>
                      <text content={item.title} fg={active() ? activeText : tuiTheme.text} />
                      <Show when={item.description}>
                        <text content={item.description ?? ""} fg={active() ? activeText : tuiTheme.textMuted} />
                      </Show>
                    </box>
                    <Show when={item.meta}>
                      <text content={item.meta ?? ""} fg={active() ? activeText : tuiTheme.textMuted} />
                    </Show>
                  </box>
                )
              }}
            </For>
          </scrollbox>
        </Show>
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
        <text content="up/down move" fg={tuiTheme.textMuted} />
        <text content="enter select" fg={tuiTheme.textMuted} />
      </box>
    </OverlaySurface>
  )
}
