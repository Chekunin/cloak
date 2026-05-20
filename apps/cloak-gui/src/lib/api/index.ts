/**
 * Single import point for the entire daemon API surface.
 *
 *     import { vault, secrets, endpoints, tokens, daemon } from '$lib/api';
 *
 * keeps call sites grouped by domain ("vault.unlock()", "secrets.list()").
 */
export * as daemon from './daemon';
export * as vault from './vault';
export * as secrets from './secrets';
export * as endpoints from './endpoints';
export * as exec from './exec';
export * as tokens from './tokens';
export * as audit from './audit';
export * as update from './update';

export type * from './types';
export { isCommandError } from './types';
