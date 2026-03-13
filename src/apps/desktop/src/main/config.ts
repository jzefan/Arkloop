import * as fs from 'fs'
import * as path from 'path'
import * as os from 'os'
import { DEFAULT_CONFIG } from './types'
import type { AppConfig } from './types'

const CONFIG_DIR = path.join(os.homedir(), '.arkloop')
const CONFIG_PATH = path.join(CONFIG_DIR, 'config.json')

export function loadConfig(): AppConfig {
  try {
    const raw = fs.readFileSync(CONFIG_PATH, 'utf-8')
    const parsed = JSON.parse(raw) as Partial<AppConfig>
    return { ...DEFAULT_CONFIG, ...parsed }
  } catch {
    return { ...DEFAULT_CONFIG }
  }
}

export function saveConfig(config: AppConfig): void {
  fs.mkdirSync(CONFIG_DIR, { recursive: true })
  fs.writeFileSync(CONFIG_PATH, JSON.stringify(config, null, 2), 'utf-8')
}

export function getConfigPath(): string {
  return CONFIG_PATH
}
