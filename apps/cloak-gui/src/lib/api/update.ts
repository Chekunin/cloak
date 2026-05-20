import { call } from './transport';
import type { UpdateInfo } from './types';

/**
 * Check the configured update endpoint for a newer release. Resolves to
 * `null` when the app is already on the latest version.
 */
export function checkForUpdate(): Promise<UpdateInfo | null> {
  return call<UpdateInfo | null>('check_for_update');
}

/**
 * Download and install the available update, then relaunch. On success the
 * app restarts, so this promise effectively never resolves.
 */
export function installUpdate(): Promise<void> {
  return call<void>('install_update');
}
