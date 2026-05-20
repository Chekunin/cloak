import { call } from './transport';
import type { AuditEntry } from './types';

/** Returns the latest `limit` audit-log entries, oldest first. */
export function tail(limit: number): Promise<AuditEntry[]> {
  return call<AuditEntry[]>('audit_tail', { limit });
}
