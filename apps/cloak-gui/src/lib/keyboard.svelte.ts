/**
 * Global keyboard shortcuts.
 *
 *   ⌘/Ctrl + K    Open command palette.
 *   ⌘/Ctrl + L    Lock the vault.
 *   ⌘/Ctrl + N    New secret.
 *   /             Focus the palette search (when palette is open).
 *
 * All shortcuts respect a "don't fire while typing in an input" rule. The
 * command palette uses `/` to refocus its own search, but only when it's
 * already open — the global handler bails out for non-modifier keys while
 * the user is in a text field.
 */

import { vault, isCommandError } from '$lib/api';
import { navigate, router } from './router.svelte';
import { vaultStore } from './stores/vault.svelte';
import { connection } from './stores/connection.svelte';
import { palette } from './stores/palette.svelte';
import { toasts } from './stores/toasts.svelte';

const isMac = typeof navigator !== 'undefined' && /Mac|iPhone|iPad/.test(navigator.platform);
const mod = (e: KeyboardEvent): boolean => (isMac ? e.metaKey : e.ctrlKey);

function inTextField(e: KeyboardEvent): boolean {
  const t = e.target;
  if (!(t instanceof HTMLElement)) return false;
  const tag = t.tagName;
  return tag === 'INPUT' || tag === 'TEXTAREA' || t.isContentEditable;
}

async function lockVault(): Promise<void> {
  if (vaultStore.phase.kind !== 'ok') return;
  if (vaultStore.phase.status.state !== 'unlocked') return;
  try {
    await vault.lock();
    toasts.success('Vault locked');
    await vaultStore.refresh();
    navigate('unlock');
  } catch (err) {
    const msg = isCommandError(err)
      ? err.message
      : err instanceof Error
        ? err.message
        : String(err);
    toasts.error('Could not lock vault', msg);
  }
}

/** Install the global keydown listener. Call once in App.svelte. */
export function installKeyboardShortcuts(): () => void {
  if (typeof window === 'undefined') return () => {};

  const handler = (e: KeyboardEvent) => {
    // Palette toggle is allowed everywhere — even mid-typing — so users can
    // jump quickly without first un-focusing a form.
    if (mod(e) && e.key.toLowerCase() === 'k') {
      e.preventDefault();
      palette.toggle();
      return;
    }

    // Other shortcuts: skip when typing in form fields.
    if (inTextField(e)) return;

    if (mod(e) && e.key.toLowerCase() === 'l') {
      e.preventDefault();
      void lockVault();
      return;
    }

    if (mod(e) && e.key.toLowerCase() === 'n') {
      // Only meaningful when authorised + vault unlocked.
      if (
        vaultStore.phase.kind === 'ok' &&
        vaultStore.phase.status.state === 'unlocked' &&
        connection.state.kind === 'connected' &&
        connection.state.hasToken
      ) {
        e.preventDefault();
        navigate('secrets:create');
      }
      return;
    }

    // `/` focuses the palette search if the palette is already open.
    if (e.key === '/' && palette.open) {
      e.preventDefault();
      const input = document.querySelector<HTMLInputElement>('[data-palette-input]');
      input?.focus();
    }
  };

  window.addEventListener('keydown', handler);
  // Initial pass to silence "unused" warnings on router during early load.
  void router.route.path;

  return () => window.removeEventListener('keydown', handler);
}
