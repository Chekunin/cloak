/**
 * Typed wrapper around Tauri's `invoke()`.
 *
 * All daemon traffic in the frontend goes through this one function. That
 * makes it the single place to add cross-cutting concerns later: tracing,
 * retries on transport errors, telemetry, etc.
 */

import { invoke } from '@tauri-apps/api/core';

/**
 * Call a `#[tauri::command]` and decode the result as `T`.
 *
 * Errors thrown from the Rust side are JSON objects matching
 * {@link CommandError}; we rethrow them as-is for the caller to branch on.
 */
export async function call<T>(
  name: string,
  args?: Record<string, unknown>,
): Promise<T> {
  return invoke<T>(name, args);
}
