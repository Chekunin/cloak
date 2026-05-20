import { call } from './transport';
import type { Endpoint } from './types';

export function list(): Promise<Endpoint[]> {
  return call<Endpoint[]>('endpoints_list');
}

export function open(secret: string, ttlSeconds = 0): Promise<Endpoint> {
  return call<Endpoint>('endpoints_open', { secret, ttlSeconds });
}

export function close(endpointId: string): Promise<void> {
  return call<void>('endpoints_close', { endpointId });
}
