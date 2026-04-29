// IndexedDB hot cache for workTodos, keyed by threadId.
//
// Schema:
//   DB: "arkloop-todo-cache" v1
//   Object store: "todos"
//     keyPath: "threadId"
//     value: { threadId: string; todos: TodoItem[]; updatedAt: number }
//
// All operations are best-effort. Failures (private browsing, quota exceeded, etc.)
// are silently swallowed — todo persistence is never on the critical path.

export interface TodoItem {
  id: string
  content: string
  activeForm?: string
  status: string
}

const DB_NAME = 'arkloop-todo-cache'
const DB_VERSION = 1
const STORE_NAME = 'todos'

function openDb(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const req = indexedDB.open(DB_NAME, DB_VERSION)
    req.onupgradeneeded = (e) => {
      const db = (e.target as IDBOpenDBRequest).result
      if (!db.objectStoreNames.contains(STORE_NAME)) {
        db.createObjectStore(STORE_NAME, { keyPath: 'threadId' })
      }
    }
    req.onsuccess = (e) => resolve((e.target as IDBOpenDBRequest).result)
    req.onerror = (e) => reject((e.target as IDBOpenDBRequest).error)
  })
}

export async function getThreadTodos(threadId: string): Promise<TodoItem[]> {
  try {
    const db = await openDb()
    return new Promise((resolve) => {
      const tx = db.transaction(STORE_NAME, 'readonly')
      const req = tx.objectStore(STORE_NAME).get(threadId)
      req.onsuccess = (e) => {
        const record = (e.target as IDBRequest).result as { todos: TodoItem[] } | undefined
        resolve(record?.todos ?? [])
      }
      req.onerror = () => resolve([])
    })
  } catch {
    return []
  }
}

export async function setThreadTodos(threadId: string, todos: TodoItem[]): Promise<void> {
  try {
    const db = await openDb()
    await new Promise<void>((resolve, reject) => {
      const tx = db.transaction(STORE_NAME, 'readwrite')
      tx.objectStore(STORE_NAME).put({ threadId, todos, updatedAt: Date.now() })
      tx.oncomplete = () => resolve()
      tx.onerror = (e) => reject((e.target as IDBTransaction).error)
    })
  } catch {
    // best-effort
  }
}

export async function clearThreadTodos(threadId: string): Promise<void> {
  try {
    const db = await openDb()
    await new Promise<void>((resolve) => {
      const tx = db.transaction(STORE_NAME, 'readwrite')
      tx.objectStore(STORE_NAME).delete(threadId)
      tx.oncomplete = () => resolve()
      tx.onerror = () => resolve()
    })
  } catch {
    // best-effort
  }
}
