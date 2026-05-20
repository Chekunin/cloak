/**
 * Toast notifications.
 *
 * Append-only queue with an auto-dismiss timer per toast. Components read
 * `toasts.list` reactively and `<Toast>` renders the queue. `success`,
 * `error`, and `info` are the only kinds — anything more elaborate (action
 * buttons, undo) lives on a custom component instead.
 */

export type ToastKind = 'success' | 'error' | 'info';

export interface Toast {
  id: number;
  kind: ToastKind;
  message: string;
  /** Optional clarifying hint shown in smaller text. */
  hint?: string;
}

class ToastStore {
  list: Toast[] = $state([]);
  private nextId = 1;
  /** Default time (ms) a toast lives before being auto-dismissed. */
  private autoDismissMs = 5000;

  success(message: string, hint?: string): void {
    this.push({ kind: 'success', message, hint });
  }

  error(message: string, hint?: string): void {
    this.push({ kind: 'error', message, hint });
  }

  info(message: string, hint?: string): void {
    this.push({ kind: 'info', message, hint });
  }

  dismiss(id: number): void {
    this.list = this.list.filter((t) => t.id !== id);
  }

  private push(t: Omit<Toast, 'id'>): void {
    const id = this.nextId++;
    this.list = [...this.list, { id, ...t }];
    setTimeout(() => this.dismiss(id), this.autoDismissMs);
  }
}

export const toasts = new ToastStore();
