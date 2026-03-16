import * as path from 'path'
import * as https from 'https'
import * as http from 'http'
import * as os from 'os'
import * as fs from 'fs'
import { spawn } from 'child_process'

export type VmImageStatus = 'not_installed' | 'downloading' | 'ready' | 'error' | 'unsupported' | 'custom'

export type VmImageProgress = {
  phase: 'connecting' | 'downloading' | 'extracting' | 'done' | 'error'
  percent: number
  bytesDownloaded: number
  bytesTotal: number
  error?: string
}

const VM_DIR = path.join(os.homedir(), '.arkloop', 'vm')
const VM_VERSION_FILE = path.join(VM_DIR, 'vm.version.json')
const DEFAULT_KERNEL_FILE = path.join(VM_DIR, 'vmlinux')
const DEFAULT_ROOTFS_FILE = path.join(VM_DIR, 'rootfs.ext4')
const DEFAULT_INITRD_FILE = path.join(VM_DIR, 'initramfs.gz')
const DEFAULT_DOWNLOAD_BASE = 'https://github.com/qqqqqf-q/arkloop/releases/download'

let status: VmImageStatus = 'not_installed'
let onStatusChange: ((s: VmImageStatus) => void) | null = null

export function getVmImageStatus(): VmImageStatus {
  if (process.platform !== 'darwin') return 'unsupported'
  return status
}

export function setVmImageStatusListener(fn: (s: VmImageStatus) => void): void {
  onStatusChange = fn
}

function setStatus(s: VmImageStatus): void {
  status = s
  onStatusChange?.(s)
}

function getVmAssetName(): string {
  const arch = process.arch === 'arm64' ? 'arm64' : 'x64'
  return `vm-darwin-${arch}.tar.gz`
}

export function getVmDir(): string {
  return VM_DIR
}

// Resolve the effective kernel path: custom override or default ~/.arkloop/vm/vmlinux
export function getEffectiveKernelPath(overridePath?: string): string {
  return (overridePath && overridePath.trim()) ? overridePath.trim() : DEFAULT_KERNEL_FILE
}

// Resolve the effective rootfs path: custom override or default ~/.arkloop/vm/rootfs.ext4
export function getEffectiveRootfsPath(overridePath?: string): string {
  return (overridePath && overridePath.trim()) ? overridePath.trim() : DEFAULT_ROOTFS_FILE
}

// Resolve the effective initrd path: custom override or default ~/.arkloop/vm/initramfs.gz (optional)
export function getEffectiveInitrdPath(overridePath?: string): string | null {
  if (overridePath && overridePath.trim()) return overridePath.trim()
  return fs.existsSync(DEFAULT_INITRD_FILE) ? DEFAULT_INITRD_FILE : null
}

// Legacy accessors (used by sidecar.ts when no override is configured)
export function getVmKernelPath(): string { return DEFAULT_KERNEL_FILE }
export function getVmRootfsPath(): string { return DEFAULT_ROOTFS_FILE }

// Check if VM images are available given optional custom path overrides
export function isVmImageAvailable(overrideKernelPath?: string, overrideRootfsPath?: string): boolean {
  if (process.platform !== 'darwin') return false
  try {
    const kernelPath = getEffectiveKernelPath(overrideKernelPath)
    const rootfsPath = getEffectiveRootfsPath(overrideRootfsPath)
    return fs.existsSync(kernelPath) && fs.existsSync(rootfsPath)
  } catch {
    return false
  }
}

function httpsGet(url: string, maxRedirects = 5): Promise<http.IncomingMessage> {
  return new Promise((resolve, reject) => {
    if (maxRedirects <= 0) {
      reject(new Error('too many redirects'))
      return
    }
    https.get(url, { headers: { 'User-Agent': 'arkloop-desktop' } }, (res) => {
      if (res.statusCode && res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
        res.resume()
        httpsGet(res.headers.location, maxRedirects - 1).then(resolve, reject)
        return
      }
      resolve(res)
    }).on('error', reject)
  })
}

export async function checkVmImageVersion(): Promise<{
  current: string | null
  latest: string | null
  updateAvailable: boolean
}> {
  let current: string | null = null
  try {
    const raw = fs.readFileSync(VM_VERSION_FILE, 'utf-8')
    current = JSON.parse(raw).version ?? null
  } catch {}

  let latest: string | null = null
  try {
    const res = await httpsGet('https://api.github.com/repos/qqqqqf-q/arkloop/releases/latest')
    const body = await new Promise<string>((resolve, reject) => {
      const chunks: Buffer[] = []
      res.on('data', (c: Buffer) => chunks.push(c))
      res.on('end', () => resolve(Buffer.concat(chunks).toString()))
      res.on('error', reject)
    })
    if (res.statusCode === 200) {
      const data = JSON.parse(body)
      latest = data.tag_name?.replace(/^v/, '') ?? null
    }
  } catch {
    return { current, latest: null, updateAvailable: false }
  }

  const updateAvailable = !!(latest && latest !== current)
  return { current, latest, updateAvailable }
}

function extractTarball(tarballPath: string, destDir: string): Promise<void> {
  return new Promise((resolve, reject) => {
    const child = spawn('tar', ['-xzf', tarballPath, '-C', destDir], {
      stdio: ['ignore', 'pipe', 'pipe'],
    })
    child.on('error', reject)
    child.on('exit', (code) => {
      if (code === 0) resolve()
      else reject(new Error(`tar extraction failed with code ${code}`))
    })
  })
}

export async function downloadVmImages(
  onProgress?: (progress: VmImageProgress) => void,
): Promise<void> {
  if (process.platform !== 'darwin') {
    throw new Error('Apple VM images are only available on macOS')
  }
  if (status === 'downloading') {
    throw new Error('VM image download already in progress')
  }

  const emit = (p: VmImageProgress) => onProgress?.(p)
  const tmpPath = path.join(os.homedir(), '.arkloop', `vm-download-${Date.now()}-${process.pid}.tar.gz`)

  emit({ phase: 'connecting', percent: 0, bytesDownloaded: 0, bytesTotal: 0 })
  setStatus('downloading')

  try {
    const releaseRes = await httpsGet('https://api.github.com/repos/qqqqqf-q/arkloop/releases/latest')
    const releaseBody = await new Promise<string>((resolve, reject) => {
      const chunks: Buffer[] = []
      releaseRes.on('data', (c: Buffer) => chunks.push(c))
      releaseRes.on('end', () => resolve(Buffer.concat(chunks).toString()))
      releaseRes.on('error', reject)
    })
    if (releaseRes.statusCode !== 200) {
      throw new Error(`Failed to fetch release info (HTTP ${releaseRes.statusCode}). Check that GitHub release assets exist at github.com/qqqqqf-q/arkloop/releases.`)
    }
    const release = JSON.parse(releaseBody)
    const version = (release.tag_name as string)?.replace(/^v/, '')
    if (!version) throw new Error('Invalid release response: missing tag_name')

    const downloadBase = process.env.ARKLOOP_VM_DOWNLOAD_URL || DEFAULT_DOWNLOAD_BASE
    const assetName = getVmAssetName()
    const url = `${downloadBase}/v${version}/${assetName}`

    fs.mkdirSync(path.dirname(tmpPath), { recursive: true })

    const dlRes = await httpsGet(url)
    if (dlRes.statusCode !== 200) {
      dlRes.resume()
      throw new Error(`Download failed (HTTP ${dlRes.statusCode}) for asset: ${assetName}`)
    }

    const bytesTotal = parseInt(dlRes.headers['content-length'] || '0', 10)
    let bytesDownloaded = 0

    emit({ phase: 'downloading', percent: 0, bytesDownloaded: 0, bytesTotal })

    const ws = fs.createWriteStream(tmpPath)
    await new Promise<void>((resolve, reject) => {
      dlRes.on('data', (chunk: Buffer) => {
        bytesDownloaded += chunk.length
        const percent = bytesTotal > 0 ? Math.round((bytesDownloaded / bytesTotal) * 100) : 0
        emit({ phase: 'downloading', percent, bytesDownloaded, bytesTotal })
      })
      dlRes.pipe(ws)
      ws.on('finish', resolve)
      ws.on('error', reject)
      dlRes.on('error', reject)
    })

    emit({ phase: 'extracting', percent: 100, bytesDownloaded, bytesTotal })

    fs.mkdirSync(VM_DIR, { recursive: true })
    await extractTarball(tmpPath, VM_DIR)

    try { fs.unlinkSync(tmpPath) } catch {}

    fs.writeFileSync(VM_VERSION_FILE, JSON.stringify({
      version,
      downloadedAt: new Date().toISOString(),
      platform: process.platform,
      arch: process.arch,
    }))

    setStatus('ready')
    emit({ phase: 'done', percent: 100, bytesDownloaded, bytesTotal })
  } catch (err) {
    try { fs.unlinkSync(tmpPath) } catch {}
    setStatus('error')
    const message = err instanceof Error ? err.message : String(err)
    emit({ phase: 'error', percent: 0, bytesDownloaded: 0, bytesTotal: 0, error: message })
    throw err
  }
}

/**
 * Install VM images from existing local paths (for development / advanced use).
 * This does NOT copy the files — it validates they exist and writes a version marker
 * so the app recognises them as "ready".  The config's vmKernelPath / vmRootfsPath
 * fields are what the sidecar will actually use at runtime.
 */
export async function installLocalVmImages(
  kernelPath: string,
  rootfsPath: string,
  initrdPath?: string,
): Promise<void> {
  if (!fs.existsSync(kernelPath)) {
    throw new Error(`Kernel file not found: ${kernelPath}`)
  }
  if (!fs.existsSync(rootfsPath)) {
    throw new Error(`Rootfs file not found: ${rootfsPath}`)
  }
  if (initrdPath && initrdPath.trim() && !fs.existsSync(initrdPath)) {
    throw new Error(`Initrd file not found: ${initrdPath}`)
  }
  // No files are copied — the paths in the config point directly to the originals.
  // We just confirm they are accessible and mark status as ready.
  setStatus('custom')
}

export async function deleteVmImages(): Promise<void> {
  try {
    fs.rmSync(VM_DIR, { recursive: true, force: true })
  } catch {}
  setStatus('not_installed')
}

// Initialise status at startup
if (process.platform === 'darwin') {
  if (isVmImageAvailable()) {
    status = 'ready'
  }
}
