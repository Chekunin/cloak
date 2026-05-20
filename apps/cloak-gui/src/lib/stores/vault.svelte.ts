/**
 * Vault status store. Polls `vault.status` and exposes a reactive snapshot
 * the dashboard binds to.
 *
 * Polling auto-pauses while the daemon is disconnected; the connection store
 * is the upstream signal for that.
 */

import { vault, isCommandError } from '$lib/api';
import type { VaultStatus } from '$lib/api';

export type VaultPhase =
  | { kind: 'loading' }
  | { kind: 'ok'; status: VaultStatus }
  | { kind: 'error'; code: string; message: string; hint?: string };

class VaultStore {
  phase: VaultPhase = $state({ kind: 'loading' });

  private intervalMs = 1500;
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
      const status = await vault.status();
      this.phase = { kind: 'ok', status };
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

export const vaultStore = new VaultStore();
