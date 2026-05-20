/**
 * Helpers shared by the per-type forms.
 *
 * Building the request bodies takes a little massaging — the form holds ports
 * as strings (so empty input means "leave blank"), and `endpoint_config`
 * lives inside the same form state to make binding tidy. These helpers
 * convert the form view back into the request shape the daemon expects.
 */

import type { EndpointConfig, EndpointMode } from '$lib/api';

export interface EndpointFormState {
  mode: EndpointMode;
  persistent_port_str: string;
  session_ttl_str: string;
  require_local_auth: boolean;
}

export const defaultEndpointForm = (): EndpointFormState => ({
  mode: 'persistent',
  persistent_port_str: '',
  session_ttl_str: '3600',
  require_local_auth: true,
});

export function buildEndpointConfig(state: EndpointFormState): EndpointConfig {
  const out: EndpointConfig = {
    mode: state.mode,
    require_local_auth: state.require_local_auth,
  };
  if (state.mode === 'persistent' && state.persistent_port_str.trim()) {
    const p = Number.parseInt(state.persistent_port_str, 10);
    if (Number.isFinite(p)) out.persistent_port = p;
  }
  if (state.mode === 'session' && state.session_ttl_str.trim()) {
    const t = Number.parseInt(state.session_ttl_str, 10);
    if (Number.isFinite(t)) out.session_ttl_seconds = t;
  }
  return out;
}

/** Validates the endpoint-config inputs. Empty result = OK. */
export function validateEndpointConfig(
  state: EndpointFormState,
): Partial<Record<'persistentPort' | 'sessionTtl', string>> {
  const errors: Partial<Record<'persistentPort' | 'sessionTtl', string>> = {};
  if (state.mode === 'persistent' && state.persistent_port_str.trim()) {
    const p = Number.parseInt(state.persistent_port_str, 10);
    if (!Number.isFinite(p) || p <= 0 || p > 65535) {
      errors.persistentPort = 'Port must be 1–65535 or blank.';
    }
  }
  if (state.mode === 'session' && state.session_ttl_str.trim()) {
    const t = Number.parseInt(state.session_ttl_str, 10);
    if (!Number.isFinite(t) || t <= 0) {
      errors.sessionTtl = 'TTL must be a positive integer.';
    }
  }
  return errors;
}

/** Common error-message extraction for create / update calls. */
export function extractErrorMessage(err: unknown): string {
  if (err && typeof err === 'object' && 'message' in err) {
    const m = (err as { message: unknown }).message;
    const h = (err as { hint?: unknown }).hint;
    if (typeof m === 'string') {
      return typeof h === 'string' ? `${m} — ${h}` : m;
    }
  }
  if (err instanceof Error) return err.message;
  return String(err);
}
