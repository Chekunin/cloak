/**
 * In-app update store. Drives the "Check for Updates" flow: a check against
 * the GitHub Releases endpoint, then an optional download-install-relaunch.
 *
 * `phase` is `null` while the update dialog is closed; any other value means
 * the dialog is showing that state. Triggered from the tray menu and the
 * command palette.
 */

import { update as updateApi, isCommandError } from '$lib/api';
import type { UpdateInfo } from '$lib/api';

export type UpdatePhase =
  | { kind: 'checking' }
  | { kind: 'uptodate' }
  | { kind: 'available'; info: UpdateInfo }
  | { kind: 'installing' }
  | { kind: 'error'; message: string };

class UpdateStore {
  /** Current dialog state, or `null` when the dialog is dismissed. */
  phase = $state<UpdatePhase | null>(null);

  /** Check for a newer release and open the dialog with the result. */
  async check(): Promise<void> {
    if (this.phase?.kind === 'checking' || this.phase?.kind === 'installing') return;
    this.phase = { kind: 'checking' };
    try {
      const info = await updateApi.checkForUpdate();
      this.phase = info ? { kind: 'available', info } : { kind: 'uptodate' };
    } catch (err) {
      this.phase = { kind: 'error', message: errorMessage(err) };
    }
  }

  /** Download, install, and relaunch into the update. */
  async install(): Promise<void> {
    this.phase = { kind: 'installing' };
    try {
      await updateApi.installUpdate();
      // On success the app relaunches — control never returns here.
    } catch (err) {
      this.phase = { kind: 'error', message: errorMessage(err) };
    }
  }

  /** Close the dialog. */
  dismiss(): void {
    this.phase = null;
  }
}

function errorMessage(err: unknown): string {
  if (isCommandError(err)) {
    return err.hint ? `${err.message} — ${err.hint}` : err.message;
  }
  return err instanceof Error ? err.message : String(err);
}

/** Shared singleton — import this directly from components. */
export const update = new UpdateStore();
