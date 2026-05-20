import { call } from './transport';
import type {
  CreateSecretRequest,
  RevealedSecret,
  Secret,
  UpdateSecretRequest,
} from './types';

export function list(): Promise<Secret[]> {
  return call<Secret[]>('secrets_list');
}

export function get(idOrName: string): Promise<Secret> {
  return call<Secret>('secrets_get', { idOrName });
}

/**
 * Decrypt and return one secret's material. Requires the vault master
 * password — a re-authentication gate enforced by the daemon. Every call is
 * audit-logged. Keep the result out of any polling store.
 */
export function reveal(idOrName: string, password: string): Promise<RevealedSecret> {
  return call<RevealedSecret>('secrets_reveal', { idOrName, password });
}

export function create(request: CreateSecretRequest): Promise<Secret> {
  return call<Secret>('secrets_create', { request });
}

export function update(request: UpdateSecretRequest): Promise<Secret> {
  return call<Secret>('secrets_update', { request });
}

export function remove(idOrName: string): Promise<void> {
  return call<void>('secrets_delete', { idOrName });
}
