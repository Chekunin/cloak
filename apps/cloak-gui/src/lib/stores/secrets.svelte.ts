/**
 * Polled secrets store.
 *
 * Refreshes the list every few seconds when the vault is unlocked. Tracks
 * three phases — loading / ok / error — so screens can render appropriate
 * states without re-implementing the polling pattern.
 */

import { secrets, isCommandError } from '$lib/api';
import type { Secret } from '$lib/api';

export type SecretsPhase =
  | { kind: 'loading' }
  | { kind: 'ok'; items: Secret[] }
  | { kind: 'error'; code: string; message: string; hint?: string };

class SecretsStore {
  phase: SecretsPhase = $state({ kind: 'loading' });

  private intervalMs = 3000;
  private timer: number | null = null;
  private inFlight = false;

  start(): void {
    if (this.timer !== null) return;
    void this.tick();
    this.timer = window.setInterval(() => void this.tick(), this.intervalMs);
  }

  stop(): void {
    if (this.timer !== null) {
      window.clearInterval(this.timer);
      this.timer = null;
    }
  }

  async refresh(): Promise<void> {
    await this.tick();
  }

  private async tick(): Promise<void> {
    if (this.inFlight) return;
    this.inFlight = true;
    try {
      const items = await secrets.list();
      this.phase = { kind: 'ok', items };
    } catch (err) {
      if (isCommandError(err)) {
        this.phase = {
          kind: 'error',
          code: err.code,
          message: err.message,
          hint: err.hint,
        };
      } else {
        this.phase = {
          kind: 'error',
          code: 'internal_error',
          message: err instanceof Error ? err.message : String(err),
        };
      }
    } finally {
      this.inFlight = false;
    }
  }
}

export const secretsStore = new SecretsStore();
