import type { BrowserContext } from 'playwright';
import type { StorageClient } from '../storage/minio-client.js';

export interface BrowserPoolConfig {
  maxBrowsers: number;
  maxContextsPerBrowser: number;
  contextIdleTimeoutMs: number;
  contextMaxLifetimeMs: number;
  storage: StorageClient;
}

export interface ActiveContext {
  context: BrowserContext;
  sessionId: string;
  orgId: string;
  lastActive: number;
  createdAt: number;
  idleTimer: NodeJS.Timeout | null;
}

export interface ContextHandle {
  context: BrowserContext;
  sessionId: string;
  orgId: string;
}

// BrowserPool 管理 Chromium 实例和 BrowserContext 生命周期。
// 完整实现在 AS-7.2 中完成。
export class BrowserPool {
  private readonly config: BrowserPoolConfig;

  constructor(config: BrowserPoolConfig) {
    this.config = config;
  }

  // getContext 按 sessionId 返回复用或新建的 BrowserContext。
  async getContext(orgId: string, sessionId: string): Promise<ContextHandle> {
    throw new Error(`BrowserPool.getContext not implemented (AS-7.2): ${orgId}/${sessionId}`);
  }

  // releaseContext 触发 idle timer，idle 超时后持久化并关闭。
  releaseContext(sessionId: string): void {
    throw new Error(`BrowserPool.releaseContext not implemented (AS-7.2): ${sessionId}`);
  }

  // shutdown 关闭所有活跃 context 并关闭浏览器实例。
  async shutdown(): Promise<void> {
    throw new Error('BrowserPool.shutdown not implemented (AS-7.2)');
  }

  get poolConfig(): Readonly<BrowserPoolConfig> {
    return this.config;
  }
}
