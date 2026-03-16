import * as path from 'path'
import * as https from 'https'
import * as http from 'http'
import * as os from 'os'
import * as fs from 'fs'
import { spawn } from 'child_process'

export type RootfsStatus = 'not_installed' | 'downloading' | 'ready' | 'error'

export type RootfsProgress = {
  phase: 'connecting' | 'downloading' | 'extracting' | 'done' | 'error'
  percent: number
  bytesDownloaded: number
  bytesTotal: number
  error?: string
}

const ROOTFS_DIR = path.join(os.homedir(), '.arkloop', 'rootfs')
const ROOTFS_VERSION_FILE = path.join(ROOTFS_DIR, 'rootfs.version.json')
const DEFAULT_DOWNLOAD_BASE = 'https://github.com/qqqqqf-q/arkloop/releases/download'

let status: RootfsStatus = 'not_installed'
let onStatusChange: ((s: RootfsStatus) => void) | null = null

export function getRootfsStatus(): RootfsStatus {
  return status
}

export function setRootfsStatusListener(fn: (s: RootfsStatus) => void): void {
  onStatusChange = fn
}

function setStatus(s: RootfsStatus): void {
  status = s
  onStatusChange?.(s)
}

function getRootfsAssetName(): string {
  const platform = process.platform
  const arch = process.arch === 'arm64' ? 'arm64' : 'x64'
  return `rootfs-${platform}-${arch}.tar.gz`
}

export function getRootfsPath(): string {
  return ROOTFS_DIR
}

export function isRootfsAvailable(): boolean {
  try {
    const versionExists = fs.existsSync(ROOTFS_VERSION_FILE)
    const dirExists = fs.existsSync(ROOTFS_DIR)
    return versionExists && dirExists
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

export async function checkRootfsVersion(): Promise<{
  current: string | null
  latest: string | null
  updateAvailable: boolean
}> {
  let current: string | null = null
  try {
    const raw = fs.readFileSync(ROOTFS_VERSION_FILE, 'utf-8')
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

export async function downloadRootfs(
  onProgress?: (progress: RootfsProgress) => void,
): Promise<void> {
  if (status === 'downloading') {
    throw new Error('rootfs download already in progress')
  }
  const emit = (p: RootfsProgress) => onProgress?.(p)
  const tmpPath = path.join(os.homedir(), '.arkloop', `rootfs-download-${Date.now()}-${process.pid}.tar.gz`)

  emit({ phase: 'connecting', percent: 0, bytesDownloaded: 0, bytesTotal: 0 })
  setStatus('downloading')

  try {
    // 获取最新版本
    const releaseRes = await httpsGet('https://api.github.com/repos/qqqqqf-q/arkloop/releases/latest')
    const releaseBody = await new Promise<string>((resolve, reject) => {
      const chunks: Buffer[] = []
      releaseRes.on('data', (c: Buffer) => chunks.push(c))
      releaseRes.on('end', () => resolve(Buffer.concat(chunks).toString()))
      releaseRes.on('error', reject)
    })
    if (releaseRes.statusCode !== 200) {
      throw new Error(`failed to fetch release info: ${releaseRes.statusCode}`)
    }
    const release = JSON.parse(releaseBody)
    const version = (release.tag_name as string)?.replace(/^v/, '')
    if (!version) throw new Error('invalid release: missing tag_name')

    const downloadBase = process.env.ARKLOOP_ROOTFS_DOWNLOAD_URL || DEFAULT_DOWNLOAD_BASE
    const assetName = getRootfsAssetName()
    const url = `${downloadBase}/v${version}/${assetName}`

    fs.mkdirSync(path.dirname(tmpPath), { recursive: true })

    // 下载 tarball
    const dlRes = await httpsGet(url)
    if (dlRes.statusCode !== 200) {
      dlRes.resume()
      throw new Error(`download failed: ${dlRes.statusCode}`)
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

    // 解压到 rootfs 目录
    emit({ phase: 'extracting', percent: 100, bytesDownloaded, bytesTotal })

    fs.mkdirSync(ROOTFS_DIR, { recursive: true })
    await extractTarball(tmpPath, ROOTFS_DIR)

    // 清理临时文件
    try { fs.unlinkSync(tmpPath) } catch {}

    // 写入版本信息
    fs.writeFileSync(ROOTFS_VERSION_FILE, JSON.stringify({
      version,
      downloadedAt: new Date().toISOString(),
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

export async function deleteRootfs(): Promise<void> {
  try {
    fs.rmSync(ROOTFS_DIR, { recursive: true, force: true })
  } catch {}
  setStatus('not_installed')
}

// 初始化状态：如果 rootfs 已存在则设为 ready
if (isRootfsAvailable()) {
  status = 'ready'
}
