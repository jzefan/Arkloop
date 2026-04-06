import { ChatSessionProvider } from '../contexts/chat-session'
import { RunLifecycleProvider } from '../contexts/run-lifecycle'
import { MessageStoreProvider } from '../contexts/message-store'
import { MessageMetaProvider } from '../contexts/message-meta'
import { StreamProvider } from '../contexts/stream'
import { PanelProvider } from '../contexts/panels'
import { ChatView } from './ChatView'

export function ChatShell() {
  return (
    <ChatSessionProvider>
      <RunLifecycleProvider>
        <MessageStoreProvider>
          <MessageMetaProvider>
            <StreamProvider>
              <PanelProvider>
                <ChatView />
              </PanelProvider>
            </StreamProvider>
          </MessageMetaProvider>
        </MessageStoreProvider>
      </RunLifecycleProvider>
    </ChatSessionProvider>
  )
}
