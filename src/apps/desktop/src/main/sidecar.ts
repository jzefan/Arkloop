import { ChildProcess, spawn } from 'child_process'
import * as path from 'path'
import * as http from 'http'
import { app } from 'electron'

export type SidecarStatus = 'stopped' | 'starting' | 'running' | 'crashed'

const HEALTH_POLL_MS = 500
const HEALTH_TIMEOUT_MS = 30_000
const MAX_RESTARTS = 3

let proc: ChildProcess | null = null
let status: SidecarStatus = 'stopped'
let restartCount = 0
let onStatusChange: ((s: SidecarStatus) => void) | null = null

export function getSidecarStatus(): SidecarStatus {
  return status
}

export function setStatusListener(fn: (s: SidecarStatus) => void): void {
  onStatusChange = fn
}

function setStatus(s: SidecarStatus): void {
  status = s
  onStatusChange?.(s)
}

function resolveBinaryPath(): string {
  const isPackaged = app.isPackaged
  if (isPackaged) {
    return path.join(process.resourcesPath, 'sidecar', 'desktop')
  }
  // dev: 从 Go 构建产物读取
  return path.resolve(
    __dirname,
    '..', '..', '..', '..', 'services', 'desktop', 'bin', 'desktop',
  )
}

function healthCheck(port: number): Promise<boolean> {
  return new Promise((resolve) => {
    const req = http.get(`http://127.0.0.1:${port}/healthz`, (res) => {
      resolve(res.statusCode === 200)
    })
    req.on('error', () => resolve(false))
    req.setTimeout(2000, () => {
      req.destroy()
      resolve(false)
    })
  })
}

async function waitForHealthy(port: number): Promise<boolean> {
  const deadline = Date.now() + HEALTH_TIMEOUT_MS
  while (Date.now() < deadline) {
    if (await healthCheck(port)) return true
    await new Promise((r) => setTimeout(r, HEALTH_POLL_MS))
  }
  return false
}

export async function startSidecar(port: number): Promise<void> {
  if (proc) return

  const binPath = resolveBinaryPath()
  setStatus('starting')

  proc = spawn(binPath, [], {
    env: {
      ...process.env,
      ARKLOOP_API_GO_ADDR: `127.0.0.1:${port}`,
    },
    stdio: ['ignore', 'pipe', 'pipe'],
  })

  proc.stdout?.on('data', (chunk: Buffer) => {
    process.stdout.write(`[sidecar] ${chunk.toString()}`)
  })
  proc.stderr?.on('data', (chunk: Buffer) => {
    process.stderr.write(`[sidecar] ${chunk.toString()}`)
  })

  proc.on('exit', (code) => {
    proc = null
    if (status === 'stopped') return
    console.error(`sidecar exited: code=${code}`)
    if (restartCount < MAX_RESTARTS) {
      restartCount++
      setStatus('crashed')
      setTimeout(() => startSidecar(port), 1000)
    } else {
      setStatus('crashed')
    }
  })

  const ok = await waitForHealthy(port)
  if (ok) {
    restartCount = 0
    setStatus('running')
  } else {
    setStatus('crashed')
    stopSidecar()
  }
}

export function stopSidecar(): Promise<void> {
  return new Promise((resolve) => {
    if (!proc) {
      setStatus('stopped')
      resolve()
      return
    }
    setStatus('stopped')
    const p = proc
    proc = null

    const killTimer = setTimeout(() => {
      try { p.kill('SIGKILL') } catch {}
      resolve()
    }, 5000)

    p.on('exit', () => {
      clearTimeout(killTimer)
      resolve()
    })

    try { p.kill('SIGTERM') } catch {}
  })
}
