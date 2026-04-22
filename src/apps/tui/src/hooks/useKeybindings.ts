import { useKeyboard } from "@opentui/solid"
import { overlay, setOverlay, setCurrentThreadId } from "../store/app"
import { clearChat } from "../store/chat"

export function useKeybindings() {
  useKeyboard((key) => {
    // overlay open, let overlay handle its own keys
    if (overlay()) return

    if (key.ctrl && key.name === "m") {
      setOverlay("model")
    } else if (key.ctrl && key.name === "t") {
      setOverlay("session")
    } else if (key.ctrl && key.name === "n") {
      clearChat()
      setCurrentThreadId(null)
    }
  })
}
