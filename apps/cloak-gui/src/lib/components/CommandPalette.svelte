<script lang="ts">
  /**
   * Command palette — ⌘K overlay with fuzzy-search over a list of jumps and
   * quick actions.
   *
   * Keyboard:
   *   - ↑/↓     navigate
   *   - Enter   activate
   *   - Esc     close
   *   - any printable key types into the search input
   *
   * The palette is intentionally simple. No history, no scoped scopes, no
   * recently-used boosting. Add those when there's a concrete pain point.
   */

  import { vault, isCommandError } from '$lib/api';
  import { navigate } from '$lib/router.svelte';
  import { vaultStore } from '$lib/stores/vault.svelte';
  import { connection } from '$lib/stores/connection.svelte';
  import { palette } from '$lib/stores/palette.svelte';
  import { toasts } from '$lib/stores/toasts.svelte';

  interface Command {
    id: string;
    label: string;
    hint?: string;
    /** Filter expression: lowercased label + hint joined for matching. */
    search: string;
    /** Returns true while this command is meaningful. */
    enabled: () => boolean;
    run: () => void | Promise<void>;
  }

  const unlocked = $derived(
    vaultStore.phase.kind === 'ok' && vaultStore.phase.status.state === 'unlocked',
  );
  const authorised = $derived(connection.state.kind === 'connected' && connection.state.hasToken);

  const commands: Command[] = [
    {
      id: 'nav.dashboard',
      label: 'Go to Dashboard',
      hint: 'Vault status and live counters.',
      search: 'go to dashboard home',
      enabled: () => true,
      run: () => navigate('dashboard'),
    },
    {
      id: 'nav.secrets',
      label: 'Go to Secrets',
      hint: 'Stored credentials.',
      search: 'go to secrets credentials',
      enabled: () => true,
      run: () => navigate('secrets'),
    },
    {
      id: 'nav.endpoints',
      label: 'Go to Endpoints',
      hint: 'Live listeners.',
      search: 'go to endpoints listeners',
      enabled: () => true,
      run: () => navigate('endpoints'),
    },
    {
      id: 'nav.tokens',
      label: 'Go to Tokens',
      hint: 'Bearer credentials for IPC clients.',
      search: 'go to tokens',
      enabled: () => true,
      run: () => navigate('tokens'),
    },
    {
      id: 'nav.audit',
      label: 'Go to Audit log',
      hint: 'Hash-chained events.',
      search: 'go to audit log events',
      enabled: () => true,
      run: () => navigate('audit'),
    },
    {
      id: 'act.new-secret',
      label: 'New secret',
      hint: 'Open the Add Secret wizard.',
      search: 'new secret create add',
      enabled: () => unlocked && authorised,
      run: () => navigate('secrets:create'),
    },
    {
      id: 'act.lock',
      label: 'Lock vault',
      hint: 'Zero the DEK and close every endpoint.',
      search: 'lock vault',
      enabled: () => unlocked,
      run: async () => {
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
      },
    },
  ];

  let query = $state('');
  let activeIdx = $state(0);

  const filtered = $derived.by(() => {
    const q = query.trim().toLowerCase();
    const enabled = commands.filter((c) => c.enabled());
    if (!q) return enabled;
    return enabled.filter((c) => c.search.includes(q) || c.label.toLowerCase().includes(q));
  });

  // Keep the highlighted index in bounds whenever the filtered list changes.
  $effect(() => {
    if (activeIdx >= filtered.length) activeIdx = 0;
  });

  $effect(() => {
    if (palette.open) {
      query = '';
      activeIdx = 0;
      queueMicrotask(() => {
        document.querySelector<HTMLInputElement>('[data-palette-input]')?.focus();
      });
    }
  });

  function onKeyDown(e: KeyboardEvent) {
    if (!palette.open) return;
    if (e.key === 'Escape') {
      e.preventDefault();
      palette.hide();
    } else if (e.key === 'ArrowDown') {
      e.preventDefault();
      if (filtered.length > 0) activeIdx = (activeIdx + 1) % filtered.length;
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      if (filtered.length > 0) activeIdx = (activeIdx - 1 + filtered.length) % filtered.length;
    } else if (e.key === 'Enter') {
      e.preventDefault();
      const cmd = filtered[activeIdx];
      if (cmd) {
        palette.hide();
        void cmd.run();
      }
    }
  }
</script>

<svelte:window onkeydown={onKeyDown} />

{#if palette.open}
  <!-- z-40: below ConfirmDialog (z-50) so a dialog opened over it wins; below toasts (z-60). -->
  <div
    class="fixed inset-0 z-40 flex items-start justify-center bg-zinc-950/50 pt-24 backdrop-blur-sm"
    role="presentation"
    onclick={(e) => {
      if (e.target === e.currentTarget) palette.hide();
    }}
    onkeydown={() => {}}
  >
    <div
      role="dialog"
      aria-modal="true"
      aria-label="Command palette"
      class="w-full max-w-xl overflow-hidden rounded-xl border border-zinc-200 bg-white shadow-2xl dark:border-zinc-800 dark:bg-zinc-900"
    >
      <div class="border-b border-zinc-200 px-4 py-3 dark:border-zinc-800">
        <input
          type="text"
          bind:value={query}
          placeholder="Type a command or page name…"
          autocomplete="off"
          spellcheck="false"
          data-palette-input
          class="w-full select-text bg-transparent text-sm text-zinc-900 placeholder:text-zinc-400 focus:outline-none dark:text-zinc-100 dark:placeholder:text-zinc-500"
        />
      </div>

      {#if filtered.length === 0}
        <div class="px-4 py-6 text-center text-sm text-zinc-500 dark:text-zinc-400">
          No matches.
        </div>
      {:else}
        <ul class="max-h-80 overflow-y-auto py-1">
          {#each filtered as cmd, idx (cmd.id)}
            <li>
              <button
                type="button"
                onmouseenter={() => (activeIdx = idx)}
                onclick={() => {
                  palette.hide();
                  void cmd.run();
                }}
                class="
                  flex w-full items-center justify-between gap-3 px-4 py-2 text-left text-sm
                  {idx === activeIdx
                  ? 'bg-zinc-100 dark:bg-zinc-800'
                  : 'hover:bg-zinc-50 dark:hover:bg-zinc-800/50'}
                "
              >
                <span class="text-zinc-900 dark:text-zinc-100">{cmd.label}</span>
                {#if cmd.hint}
                  <span class="truncate text-xs text-zinc-500 dark:text-zinc-400">{cmd.hint}</span>
                {/if}
              </button>
            </li>
          {/each}
        </ul>
      {/if}
    </div>
  </div>
{/if}
