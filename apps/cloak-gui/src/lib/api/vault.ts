import { call } from './transport';
import type { VaultStatus } from './types';

export function status(): Promise<VaultStatus> {
  return call<VaultStatus>('vault_status');
}

export function init(password: string): Promise<void> {
  return call<void>('vault_init', { password });
}

export function unlock(password: string): Promise<void> {
  return call<void>('vault_unlock', { password });
}

export function lock(): Promise<void> {
  return call<void>('vault_lock');
}
