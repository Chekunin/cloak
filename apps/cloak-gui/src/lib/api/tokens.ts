import { call } from './transport';
import type { Token, TokenInfo } from './types';

export function list(): Promise<Token[]> {
  return call<Token[]>('tokens_list');
}

/**
 * Issue a new token. When `persist` is true, the Rust shell remembers the
 * plaintext in its AppState so subsequent dials authenticate automatically.
 *
 * The plaintext is also returned for the caller to display once. Do not
 * keep it in component state past the confirmation toast.
 */
export function create(name: string, persist: boolean): Promise<TokenInfo> {
  return call<TokenInfo>('tokens_create', { name, persist });
}

export function revoke(id: string): Promise<void> {
  return call<void>('tokens_revoke', { id });
}
