/**
 * Polled endpoints store. Refreshes the list every couple of seconds while
 * the vault is unlocked, then pauses to avoid burning IPC traffic against a
 * locked daemon.
 */

import { endpoints as endpointsApi, isCommandError } from '$lib/api';
import type { Endpoint } from '$lib/api';

export type EndpointsPhase =
  | { kind: 'loading' }
  | { kind: 'ok'; items: Endpoint[] }
  | { kind: 'error'; code: string; message: string; hint?: string };

class EndpointsStore {
  phase: EndpointsPhase = $state({ kind: 'loading' });

  private intervalMs = 2000;
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
      const items = await endpointsApi.list();
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

export const endpointsStore = new EndpointsStore();
