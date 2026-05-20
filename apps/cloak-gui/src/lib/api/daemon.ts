import { call } from './transport';
import type { DaemonInfo } from './types';

/** Returns `true` when the daemon is reachable (and authenticated, if a token is configured). */
export function ping(): Promise<boolean> {
  return call<boolean>('daemon_ping');
}

/** Returns diagnostic information about the daemon connection. */
export function info(): Promise<DaemonInfo> {
  return call<DaemonInfo>('daemon_info');
}

/**
 * Install `token` as the GUI's bearer token, validate it via `hello`. The
 * value is held in-process only — not written to disk.
 */
export function setToken(token: string): Promise<void> {
  return call<void>('gui_set_token', { token });
}

/**
 * Re-read `~/.cloak/cli_token` and adopt it as the GUI's bearer token.
 * Returns `true` when a token was found and validated. Useful when the user
 * ran `cloak token create --save` after launching the GUI.
 */
export function reloadCliToken(): Promise<boolean> {
  return call<boolean>('gui_reload_cli_token');
}
