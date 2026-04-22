import { SyntaxStyle } from "@opentui/core"
import { tuiTheme } from "../lib/theme"

const markdownSyntaxStyle = SyntaxStyle.create()

interface Props {
  role: "user" | "assistant" | "tool"
  content: string
  toolName?: string
  streaming?: boolean
}

export function MessageBubble(props: Props) {
  const isUser = () => props.role === "user"

  const roleLabel = () => {
    switch (props.role) {
      case "user": return ""
      case "assistant": return ""
      case "tool": return "Tool"
    }
  }

  const roleFg = () => {
    switch (props.role) {
      case "user": return tuiTheme.info
      case "assistant": return tuiTheme.primary
      case "tool": return tuiTheme.warning
    }
  }

  return (
    <box flexDirection="column" width="100%" paddingBottom={1}>
      {!isUser() && props.role === "tool" ? (
        <box flexDirection="row" gap={1}>
          <text content={roleLabel()} fg={roleFg()} />
          <text content={props.toolName ?? ""} fg={tuiTheme.textMuted} />
        </box>
      ) : null}
      {isUser() ? (
        <box flexDirection="row" paddingLeft={2} paddingRight={1}>
          <box flexGrow={1} backgroundColor={tuiTheme.userPromptBg} paddingLeft={1} paddingRight={1}>
            <text content={props.content} wrapMode="word" fg={tuiTheme.text} />
          </box>
        </box>
      ) : (
        <box paddingLeft={2}>
          {props.role === "assistant" ? (
            <markdown content={props.content} syntaxStyle={markdownSyntaxStyle} streaming={props.streaming} />
          ) : (
            <text
              content={props.content}
              wrapMode="word"
              fg={props.role === "tool" ? tuiTheme.textMuted : tuiTheme.text}
            />
          )}
        </box>
      )}
    </box>
  )
}
