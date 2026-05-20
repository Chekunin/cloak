<script lang="ts">
  import { tokens as tokensApi, isCommandError } from '$lib/api';
  import { connection } from '$lib/stores/connection.svelte';
  import { vaultStore } from '$lib/stores/vault.svelte';
  import { toasts } from '$lib/stores/toasts.svelte';
  import { navigate } from '$lib/router.svelte';

  /** True when we're connected, the vault is unlocked, but the GUI has no token. */
  const needsToken = $derived(
    connection.state.kind === 'connected' &&
      !connection.state.hasToken &&
      vaultStore.phase.kind === 'ok' &&
      vaultStore.phase.status.state === 'unlocked',
  );

  let bootstrapping = $state(false);

  async function onBootstrap() {
    bootstrapping = true;
    try {
      await tokensApi.create('cloak-gui', true);
      await connection.refresh();
      toasts.success("Token created", 'The GUI is now authorised.');
    } catch (err) {
      const code = isCommandError(err) ? err.code : '';
      if (code === 'unauthorized') {
        // Most likely: other tokens already exist; bootstrap path is closed.
        toasts.error(
          'Could not auto-create a token',
          'Existing tokens block the bootstrap path. Issue one manually in the Tokens tab.',
        );
        navigate('tokens');
      } else if (isCommandError(err)) {
        toasts.error('Could not create token', err.message);
      } else {
        toasts.error('Could not create token', err instanceof Error ? err.message : String(err));
      }
    } finally {
      bootstrapping = false;
    }
  }
</script>

{#if connection.state.kind === 'connecting'}
  <div
    class="rounded-md border border-zinc-200 bg-zinc-100 px-3 py-2 text-sm text-zinc-700 dark:border-zinc-800 dark:bg-zinc-900 dark:text-zinc-300"
  >
    Connecting to <code class="select-text">cloakd</code>…
  </div>
{:else if connection.state.kind === 'disconnected'}
  <div
    class="rounded-md border border-amber-300 bg-amber-50 px-3 py-2 text-sm text-amber-900 dark:border-amber-900 dark:bg-amber-950 dark:text-amber-200"
  >
    <strong>Daemon unreachable.</strong>
    {connection.state.message}. Is <code class="select-text">cloak daemon start</code> running?
  </div>
{:else if needsToken}
  <div
    class="flex items-center justify-between gap-3 rounded-md border border-amber-300 bg-amber-50 px-3 py-2 text-sm text-amber-900 dark:border-amber-900 dark:bg-amber-950 dark:text-amber-200"
  >
    <div>
      <strong>GUI is not authorised.</strong>
      Issue a client token so the daemon accepts our calls.
    </div>
    <button
      type="button"
      onclick={onBootstrap}
      disabled={bootstrapping}
      class="rounded-md bg-amber-900 px-3 py-1 text-xs font-medium text-white transition hover:bg-amber-950 disabled:opacity-50 dark:bg-amber-200 dark:text-amber-950 dark:hover:bg-amber-100"
    >
      {bootstrapping ? 'Working…' : 'Set up'}
    </button>
  </div>
{:else}
  <div
    class="rounded-md border border-emerald-300 bg-emerald-50 px-3 py-2 text-sm text-emerald-900 dark:border-emerald-900 dark:bg-emerald-950 dark:text-emerald-200"
  >
    Connected via
    <code class="select-text font-mono text-xs">{connection.state.socketPath}</code>
  </div>
{/if}
