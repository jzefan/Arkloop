// RFC1918 私有地址段及其他需要拦截的目标。
// 完整实现（含 DNS 预解析防护）在 AS-7.5 中完成。

export type BlockedReason =
  | 'private_ip'
  | 'loopback'
  | 'link_local'
  | 'cloud_metadata'
  | 'internal_service';

export interface NetworkFilterResult {
  blocked: boolean;
  reason?: BlockedReason;
  target?: string;
}

// 内网 CIDR 常量，供 AS-7.5 实现使用。
export const BLOCKED_CIDRS = [
  '10.0.0.0/8',
  '172.16.0.0/12',
  '192.168.0.0/16',
  '169.254.0.0/16',
  '127.0.0.0/8',
  '::1/128',
] as const;

export const CLOUD_METADATA_HOSTS = [
  'metadata.google.internal',
  '169.254.169.254',
] as const;

// isBlockedTarget 检查 hostname 是否落入拦截范围。
// AS-7.5 完整实现：IP 段匹配 + DNS 预解析 + 内部服务名匹配。
export function isBlockedTarget(
  _hostname: string,
  _blockedHosts: string[],
): NetworkFilterResult {
  throw new Error('isBlockedTarget not implemented (AS-7.5)');
}
