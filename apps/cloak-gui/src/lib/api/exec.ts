import { call } from './transport';
import type { ExecResult } from './types';

/**
 * Run `command` with the named secret's endpoint environment variables
 * injected — the GUI equivalent of `cloak exec --with <secret> -- <command>`.
 *
 * The daemon opens an endpoint, the command runs through the user's shell with
 * the variables layered on, and the endpoint is closed again afterwards.
 */
export function run(secret: string, command: string): Promise<ExecResult> {
  return call<ExecResult>('secrets_exec', { secret, command });
}
