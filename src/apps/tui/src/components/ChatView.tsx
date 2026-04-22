import { MessageList } from "./MessageList"
import { InputBar } from "./InputBar"

interface Props {
  onSubmit: (text: string) => void
}

export function ChatView(props: Props) {
  return (
    <box flexDirection="column" width="100%" flexGrow={1} paddingLeft={1} paddingRight={1}>
      <MessageList />
      <box paddingTop={1}>
        <InputBar onSubmit={props.onSubmit} />
      </box>
    </box>
  )
}
