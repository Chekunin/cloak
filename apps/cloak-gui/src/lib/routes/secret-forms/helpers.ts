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

// --- generic editable fields --------------------------------------------

/**
 * One editable leaf of a secret's `config` or `secret` object. Nested objects
 * are flattened to dotted paths so a single key/value editor handles every
 * secret type — the same flattening the reveal dialog uses for display.
 */
export interface EditableField {
  /** Dotted path, e.g. `inject.headers.Authorization`. */
  path: string;
  /** Editable string representation of the value. */
  value: string;
  /** How `value` is coerced back to JSON on save. */
  kind: 'string' | 'number' | 'boolean' | 'json';
  /** String leaves that arrived with newlines (e.g. a PEM) — render multi-line. */
  multiline: boolean;
}

/** Flatten an object into editable leaf fields. */
export function toEditableFields(
  obj: Record<string, unknown>,
  prefix = '',
): EditableField[] {
  const out: EditableField[] = [];
  for (const [k, v] of Object.entries(obj)) {
    const path = prefix ? `${prefix}.${k}` : k;
    if (v !== null && typeof v === 'object' && !Array.isArray(v)) {
      out.push(...toEditableFields(v as Record<string, unknown>, path));
    } else if (typeof v === 'string') {
      out.push({ path, value: v, kind: 'string', multiline: v.includes('\n') });
    } else if (typeof v === 'number') {
      out.push({ path, value: String(v), kind: 'number', multiline: false });
    } else if (typeof v === 'boolean') {
      out.push({ path, value: String(v), kind: 'boolean', multiline: false });
    } else {
      out.push({ path, value: JSON.stringify(v), kind: 'json', multiline: true });
    }
  }
  return out;
}

/** Rebuild a nested object from editable fields. Throws on invalid input. */
export function fromEditableFields(
  fields: EditableField[],
): Record<string, unknown> {
  const root: Record<string, unknown> = {};
  for (const f of fields) {
    const parts = f.path.split('.');
    let cur = root;
    for (let i = 0; i < parts.length - 1; i++) {
      const next = cur[parts[i]];
      if (typeof next !== 'object' || next === null) cur[parts[i]] = {};
      cur = cur[parts[i]] as Record<string, unknown>;
    }
    cur[parts[parts.length - 1]] = coerceField(f);
  }
  return root;
}

function coerceField(f: EditableField): unknown {
  switch (f.kind) {
    case 'string':
      return f.value;
    case 'number': {
      const n = Number(f.value);
      if (f.value.trim() === '' || !Number.isFinite(n)) {
        throw new Error(`"${f.path}" must be a number.`);
      }
      return n;
    }
    case 'boolean':
      return f.value.trim().toLowerCase() === 'true';
    case 'json':
      try {
        return JSON.parse(f.value) as unknown;
      } catch {
        throw new Error(`"${f.path}" must be valid JSON.`);
      }
  }
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
