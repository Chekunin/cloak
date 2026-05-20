/**
 * Clipboard helpers.
 *
 * Cloak's threat model treats clipboard contents as transient. When we copy
 * a secret-bearing string (connection URL, bearer token, password), we
 * schedule an auto-clear after a short window. The cancel happens if the
 * user copies something else in the meantime (their copy overwrites ours;
 * our timer is best-effort).
 */

import { toasts } from './stores/toasts.svelte';

/** Default lifetime of a sensitive clipboard payload (ms). */
const SENSITIVE_TTL_MS = 30_000;

/**
 * Copy `text` to the clipboard. When `sensitive` is true, schedule an
 * auto-clear after `SENSITIVE_TTL_MS` and toast the user.
 */
export async function copy(
  text: string,
  options: { sensitive?: boolean; label?: string } = {},
): Promise<void> {
  const { sensitive = false, label = 'Copied' } = options;
  try {
    await navigator.clipboard.writeText(text);
    if (sensitive) {
      toasts.success(`${label} to clipboard`, `Auto-clears in ${SENSITIVE_TTL_MS / 1000}s.`);
      window.setTimeout(() => {
        // Only clear if the clipboard still holds our value — don't clobber
        // whatever the user copied next.
        navigator.clipboard
          .readText()
          .then((current) => {
            if (current === text) {
              return navigator.clipboard.writeText('');
            }
            return undefined;
          })
          .catch(() => {
            /* clipboard read often denied on macOS — best-effort */
          });
      }, SENSITIVE_TTL_MS);
    } else {
      toasts.success(`${label} to clipboard`);
    }
  } catch (err) {
    toasts.error('Could not copy to clipboard', err instanceof Error ? err.message : String(err));
  }
}
