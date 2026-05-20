<script lang="ts">
  /**
   * Master-password-gated secret reveal dialog.
   *
   * Cloak normally never hands decrypted credentials to a client. Reveal is
   * the one deliberate exception — so it is gated: the daemon re-checks the
   * vault master password (a client token alone is not enough) and audit-logs
   * every call.
   *
   * This component keeps the blast radius small:
   *   - the master password is never retained past the reveal call;
   *   - revealed material lives only in local component state, never in a
   *     polling store, and is dropped on close;
   *   - the dialog auto-closes after REVEAL_TTL_MS so plaintext does not sit
   *     on screen indefinitely.
   *
   * Usage:
   *   let revealName = $state<string | null>(null);
   *   <RevealDialog secretName={revealName} onClose={() => (revealName = null)} />
   */

  import { secrets as secretsApi, isCommandError } from '$lib/api';
  import type { RevealedSecret } from '$lib/api';
  import Button from './Button.svelte';
  import PasswordInput from './PasswordInput.svelte';
  import MaskedString from './MaskedString.svelte';

  interface Props {
    /** Secret name (or id) to reveal. Non-null opens the dialog. */
    secretName: string | null;
    onClose: () => void;
  }

  const { secretName, onClose }: Props = $props();

  type Phase =
    | { kind: 'prompt' }
    | { kind: 'working' }
    | { kind: 'revealed'; data: RevealedSecret }
    | { kind: 'error'; message: string };

  /** How long revealed material stays on screen before auto-close (ms). */
  const REVEAL_TTL_MS = 30_000;

  let phase = $state<Phase>({ kind: 'prompt' });
  let password = $state('');
  let secondsLeft = $state(0);
  let dialogEl: HTMLDivElement | undefined = $state();

  let prevFocus: Element | null = null;
  let hideTimer: number | null = null;
  let countdown: number | null = null;

  function clearTimers() {
    if (hideTimer !== null) {
      window.clearTimeout(hideTimer);
      hideTimer = null;
    }
    if (countdown !== null) {
      window.clearInterval(countdown);
      countdown = null;
    }
  }

  // Reset on open; tidy up focus + timers on close.
  $effect(() => {
    if (secretName) {
      phase = { kind: 'prompt' };
      password = '';
      prevFocus = document.activeElement;
      queueMicrotask(() => dialogEl?.querySelector<HTMLInputElement>('input')?.focus());
    } else {
      clearTimers();
      if (prevFocus instanceof HTMLElement) {
        prevFocus.focus();
        prevFocus = null;
      }
    }
  });

  function close() {
    // Drop decrypted material and the password from memory before unmounting.
    phase = { kind: 'prompt' };
    password = '';
    clearTimers();
    onClose();
  }

  async function submit() {
    if (!secretName || !password || phase.kind === 'working') return;
    phase = { kind: 'working' };
    try {
      const data = await secretsApi.reveal(secretName, password);
      // Don't retain the master password past the call that needed it.
      password = '';
      phase = { kind: 'revealed', data };
      startAutoHide();
    } catch (err) {
      password = '';
      phase = { kind: 'error', message: explain(err) };
    }
  }

  function startAutoHide() {
    secondsLeft = Math.floor(REVEAL_TTL_MS / 1000);
    countdown = window.setInterval(() => {
      secondsLeft -= 1;
      if (secondsLeft <= 0) close();
    }, 1000);
  }

  function explain(err: unknown): string {
    if (isCommandError(err)) {
      return err.hint ? `${err.message} — ${err.hint}` : err.message;
    }
    return err instanceof Error ? err.message : String(err);
  }

  function onKeyDown(e: KeyboardEvent) {
    if (!secretName) return;
    if (e.key === 'Escape') {
      e.preventDefault();
      close();
    }
  }

  /**
   * Flatten a secret payload into displayable rows. Nested objects (e.g. an
   * HTTP secret's `inject.headers`) become dotted keys, so one generic
   * key/value renderer covers every secret type without per-type forms.
   */
  function flatten(
    obj: Record<string, unknown>,
    prefix = '',
  ): { key: string; value: string }[] {
    const rows: { key: string; value: string }[] = [];
    for (const [k, v] of Object.entries(obj)) {
      const key = prefix ? `${prefix}.${k}` : k;
      if (v && typeof v === 'object' && !Array.isArray(v)) {
        rows.push(...flatten(v as Record<string, unknown>, key));
      } else {
        rows.push({ key, value: typeof v === 'string' ? v : JSON.stringify(v) });
      }
    }
    return rows;
  }

  // Non-secret connection metadata (host, port, user, ...) — shown in clear.
  const configRows = $derived(
    phase.kind === 'revealed' ? flatten(phase.data.config) : [],
  );
  // Decrypted credential material — shown fully masked, reveal per field.
  const secretRows = $derived(
    phase.kind === 'revealed' ? flatten(phase.data.secret) : [],
  );
</script>

<svelte:window onkeydown={onKeyDown} />

{#if secretName}
  <div
    class="fixed inset-0 z-50 flex items-center justify-center bg-zinc-950/60 p-4 backdrop-blur-sm"
    role="presentation"
    onclick={(e) => {
      if (e.target === e.currentTarget) close();
    }}
    onkeydown={() => {}}
  >
    <div
      bind:this={dialogEl}
      role="dialog"
      aria-modal="true"
      aria-labelledby="reveal-title"
      class="w-full max-w-lg rounded-xl border border-zinc-200 bg-white shadow-xl dark:border-zinc-800 dark:bg-zinc-900"
    >
      <div class="px-6 pb-2 pt-6">
        <h2 id="reveal-title" class="text-base font-semibold text-zinc-900 dark:text-zinc-100">
          Reveal secret “{secretName}”
        </h2>
        <p class="mt-1 text-sm text-zinc-600 dark:text-zinc-400">
          {#if phase.kind === 'revealed'}
            Decrypted credential. Hidden again in {secondsLeft}s.
          {:else}
            Enter the vault master password. This reveal is recorded in the audit log.
          {/if}
        </p>
      </div>

      {#if phase.kind === 'revealed'}
        <div class="flex max-h-[60vh] flex-col gap-3 overflow-y-auto px-6 pb-4">
          {#each configRows as row (`c:${row.key}`)}
            <div class="flex flex-col gap-1">
              <span class="font-mono text-xs text-zinc-500 dark:text-zinc-400">{row.key}</span>
              <MaskedString
                value={row.value}
                sensitive={false}
                label={`${row.key} copied`}
              />
            </div>
          {/each}
          {#each secretRows as row (`s:${row.key}`)}
            <div class="flex flex-col gap-1">
              <span class="font-mono text-xs text-zinc-500 dark:text-zinc-400">{row.key}</span>
              <MaskedString value={row.value} fullMask label={`${row.key} copied`} />
            </div>
          {/each}
          {#if configRows.length === 0 && secretRows.length === 0}
            <p class="text-sm text-zinc-500 dark:text-zinc-400">
              This secret has no stored material.
            </p>
          {/if}
        </div>
      {:else}
        <div class="px-6 pb-4">
          <label
            for="reveal-password"
            class="text-xs font-medium text-zinc-700 dark:text-zinc-300"
          >
            Master password
          </label>
          <div class="mt-1.5">
            <PasswordInput
              id="reveal-password"
              bind:value={password}
              disabled={phase.kind === 'working'}
              onkeydown={(e) => {
                if (e.key === 'Enter') {
                  e.preventDefault();
                  void submit();
                }
              }}
            />
          </div>
          {#if phase.kind === 'error'}
            <p class="mt-2 text-xs text-rose-600 dark:text-rose-400">{phase.message}</p>
          {/if}
        </div>
      {/if}

      <div
        class="flex items-center justify-end gap-2 rounded-b-xl border-t border-zinc-200 bg-zinc-50/60 px-6 py-3 dark:border-zinc-800 dark:bg-zinc-900/40"
      >
        <Button variant="ghost" onclick={close}>
          {phase.kind === 'revealed' ? 'Done' : 'Cancel'}
        </Button>
        {#if phase.kind !== 'revealed'}
          <Button
            variant="primary"
            onclick={() => void submit()}
            disabled={!password || phase.kind === 'working'}
          >
            {phase.kind === 'working' ? 'Revealing…' : 'Reveal'}
          </Button>
        {/if}
      </div>
    </div>
  </div>
{/if}
