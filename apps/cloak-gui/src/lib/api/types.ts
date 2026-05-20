/**
 * TypeScript mirrors of the Cloak wire types.
 *
 * Source of truth: `pkg/client/types.go` (Go) and
 * `apps/cloak-gui/src-tauri/src/client/types.rs` (Rust). All three must stay
 * in lockstep; the contract test in `tools/contract-tests/` is the
 * tripwire.
 *
 * When the schema grows, add the field in all three places in one PR.
 */

// --- enums ---------------------------------------------------------------

export type SecretType = 'ssh' | 'postgres' | 'mysql' | 'http' | 'env';
export type EndpointMode = 'persistent' | 'session';

/** Endpoint lifecycle kind: a network listener, or an injected-values handle. */
export type EndpointKind = 'listener' | 'materialized';

// --- endpoint config -----------------------------------------------------

export interface EndpointConfig {
  mode?: EndpointMode | null;
  persistent_port?: number;
  session_ttl_seconds?: number;
  require_local_auth?: boolean;
  max_concurrent_connections?: number;
}

// --- secrets -------------------------------------------------------------

export interface Secret {
  id: string;
  name: string;
  type: SecretType;
  description?: string;
  /** Free-form non-secret config (host, port, ...). */
  config: Record<string, unknown>;
  endpoint_config: EndpointConfig;
  /** RFC3339 timestamp. */
  created_at: string;
  /** RFC3339 timestamp. */
  updated_at: string;
  /** RFC3339 timestamp or null. */
  last_used_at?: string | null;
}

// --- endpoints -----------------------------------------------------------

export interface EndpointStats {
  bytes_in: number;
  bytes_out: number;
  connections_open: number;
  connections_total: number;
  last_activity?: string | null;
}

export interface Endpoint {
  id: string;
  secret_id: string;
  secret_name: string;
  type: SecretType;
  /** 'listener' for proxied endpoints, 'materialized' for env secrets. */
  kind?: EndpointKind;
  mode: EndpointMode;
  local_addr: string;
  connection_string: string;
  env_vars?: Record<string, string>;
  opened_at: string;
  expires_at?: string | null;
  stats: EndpointStats;
}

// --- vault status --------------------------------------------------------

export type VaultState = 'uninitialized' | 'locked' | 'unlocked';

export interface VaultStatus {
  state: VaultState;
  idle_timeout_sec: number;
  expires_at?: string | null;
  endpoints_open: number;
}

// --- tokens --------------------------------------------------------------

export interface Token {
  id: string;
  name: string;
  created_at: string;
  last_seen_at?: string | null;
  revoked: boolean;
}

export interface TokenInfo {
  id: string;
  name: string;
  /** Plaintext token — shown exactly once. Do not persist in component state. */
  token: string;
}

// --- audit ---------------------------------------------------------------

export type AuditEntry = Record<string, unknown>;

// --- requests ------------------------------------------------------------

export interface CreateSecretRequest {
  name: string;
  type: SecretType;
  description?: string;
  config: Record<string, unknown>;
  secret: Record<string, unknown>;
  endpoint_config?: EndpointConfig;
}

export interface UpdateSecretRequest {
  id_or_name: string;
  description?: string;
  config?: Record<string, unknown>;
  secret?: Record<string, unknown>;
  endpoint_config?: EndpointConfig;
}

// --- exec ----------------------------------------------------------------

/**
 * Result of `secrets_exec` — running a command with a secret's endpoint
 * environment variables injected. Mirrors `src-tauri/src/exec.rs::ExecResult`.
 */
export interface ExecResult {
  stdout: string;
  stderr: string;
  exit_code: number;
  /** True when stdout or stderr was clamped to the capture limit. */
  truncated: boolean;
  /** Names — never values — of the variables injected into the child. */
  env_var_names: string[];
}

// --- diagnostics ---------------------------------------------------------

export interface DaemonInfo {
  socket_path: string;
  has_token: boolean;
}

// --- errors --------------------------------------------------------------

/**
 * Shape of the error object every Tauri command rejects with. Matches
 * `src-tauri/src/error.rs::AppErrorView`.
 *
 * Branch UI logic on `code` (stable string identifier), render `message` or
 * `hint` to the user.
 */
export interface CommandError {
  code: string;
  message: string;
  hint?: string;
}

/** Type-narrowing helper for `catch (err)`. */
export function isCommandError(value: unknown): value is CommandError {
  return (
    typeof value === 'object' &&
    value !== null &&
    'code' in value &&
    'message' in value &&
    typeof (value as { code: unknown }).code === 'string'
  );
}
