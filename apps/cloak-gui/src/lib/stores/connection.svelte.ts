/**
 * Connection health store. Polls `daemon_ping` on a schedule and exposes a
 * reactive `state` that the UI banners and chips bind to.
 *
 * Lives at a single shared instance (see export at the bottom). Components
 * subscribe by reading `.state`; the polling runs as long as `start()` has
 * been called and isn't stopped.
 */

import { daemon } from '$lib/api';

export type ConnectionState =
  | { kind: 'connecting' }
  | { kind: 'connected'; socketPath: string; hasToken: boolean }
  | { kind: 'disconnected'; message: string };

class ConnectionStore {
  /** Reactive state read by components. */
  state: ConnectionState = $state({ kind: 'connecting' });

  /** Polling interval in ms. 1Hz feels live without burning battery. */
  private intervalMs = 1500;
  private timer: number | null = null;
  private inFlight = false;

  /** Begin background polling. Idempotent. */
  start(): void {
    if (this.timer !== null) return;
    void this.tick();
    this.timer = window.setInterval(() => void this.tick(), this.intervalMs);
  }

  /** Stop background polling. */
  stop(): void {
    if (this.timer !== null) {
      window.clearInterval(this.timer);
      this.timer = null;
    }
  }

  /** Force one immediate refresh (e.g. after user action). */
  async refresh(): Promise<void> {
    await this.tick();
  }

  private async tick(): Promise<void> {
    if (this.inFlight) return;
    this.inFlight = true;
    try {
      const info = await daemon.info();
      const ok = await daemon.ping();
      this.state = ok
        ? { kind: 'connected', socketPath: info.socket_path, hasToken: info.has_token }
        : { kind: 'disconnected', message: 'daemon did not respond' };
    } catch (err) {
      this.state = {
        kind: 'disconnected',
        message: err instanceof Error ? err.message : String(err),
      };
    } finally {
      this.inFlight = false;
    }
  }
}

/** Shared singleton — import this directly from components. */
export const connection = new ConnectionStore();
