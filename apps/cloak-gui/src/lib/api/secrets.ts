import { call } from './transport';
import type { CreateSecretRequest, Secret, UpdateSecretRequest } from './types';

export function list(): Promise<Secret[]> {
  return call<Secret[]>('secrets_list');
}

export function get(idOrName: string): Promise<Secret> {
  return call<Secret>('secrets_get', { idOrName });
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
