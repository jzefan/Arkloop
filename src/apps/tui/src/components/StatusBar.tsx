import { connected, currentModelLabel, currentPersonaLabel, currentThreadId, tokenUsage } from "../store/app"
import { tuiTheme } from "../lib/theme"

export function StatusBar() {
  const modelText = () => currentModelLabel() || "auto"
  const personaText = () => currentPersonaLabel() || "none"
  const sessionText = () => {
    const tid = currentThreadId()
    return tid ? tid.slice(0, 8) : "new"
  }
  const tokenText = () => {
    const t = tokenUsage()
    return (t.input || t.output) ? `${t.input}/${t.output}` : "0/0"
  }
  const connText = () => connected() ? "connected" : "disconnected"
  const connFg = () => connected() ? tuiTheme.success : tuiTheme.error

  return (
    <box
      width="100%"
      height={1}
      flexDirection="row"
      paddingLeft={1}
      paddingRight={1}
      gap={1}
      border={["top"]}
      borderColor={tuiTheme.borderSubtle}
      backgroundColor={tuiTheme.panel}
    >
      <text content={personaText()} fg={tuiTheme.primary} />
      <text content="|" fg={tuiTheme.border} />
      <text content={modelText()} fg={tuiTheme.textMuted} />
      <text content="|" fg={tuiTheme.border} />
      <text content={sessionText()} fg={tuiTheme.textMuted} />
      <text content="|" fg={tuiTheme.border} />
      <text content={`tokens ${tokenText()}`} fg={tuiTheme.textMuted} />
      <text content="|" fg={tuiTheme.border} />
      <text content={connText()} fg={connFg()} />
    </box>
  )
}
