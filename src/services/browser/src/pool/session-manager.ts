import type { StorageClient } from '../storage/minio-client.js';

export interface SessionKey {
  orgId: string;
  sessionId: string;
}

export interface SessionManagerConfig {
  storage: StorageClient;
  ttlDays?: number;
}

// SessionManager 管理 storageState 的加载、持久化和删除。
// 完整实现在 AS-7.3 中完成。
export class SessionManager {
  private readonly config: SessionManagerConfig;

  constructor(config: SessionManagerConfig) {
    this.config = config;
  }

  // loadState 从 MinIO 加载 storageState；不存在时返回 null（空白 session）。
  async loadState(key: SessionKey): Promise<object | null> {
    throw new Error(`SessionManager.loadState not implemented (AS-7.3): ${key.orgId}/${key.sessionId}`);
  }

  // saveState 将 storageState 写回 MinIO（覆盖写）。
  async saveState(key: SessionKey, state: object): Promise<void> {
    throw new Error(`SessionManager.saveState not implemented (AS-7.3): ${key.orgId}/${key.sessionId}`);
  }

  // deleteState 删除 MinIO 上的 state.json 并关闭活跃 context。
  async deleteState(key: SessionKey): Promise<void> {
    throw new Error(`SessionManager.deleteState not implemented (AS-7.3): ${key.orgId}/${key.sessionId}`);
  }
}
