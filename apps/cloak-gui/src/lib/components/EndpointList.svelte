<script lang="ts">
  import { SvelteSet } from 'svelte/reactivity';
  import { navigate } from '$lib/router.svelte';
  import { endpoints as endpointsApi, isCommandError } from '$lib/api';
  import type { Endpoint } from '$lib/api';
  import { endpointsStore } from '$lib/stores/endpoints.svelte';
  import { secretsStore } from '$lib/stores/secrets.svelte';
  import { vaultStore } from '$lib/stores/vault.svelte';
  import { toasts } from '$lib/stores/toasts.svelte';
  import { formatBytes, timeAgo } from '$lib/format';
  import Button from './Button.svelte';
  import Card from './Card.svelte';
  import EmptyState from './EmptyState.svelte';
  import MaskedString from './MaskedString.svelte';
  import CopyButton from './CopyButton.svelte';

  const vaultUnlocked = $derived(
    vaultStore.phase.kind === 'ok' && vaultStore.phase.status.state === 'unlocked',
  );

  /** Tracks which endpoint cards are expanded to show env vars. */
  const expanded = new SvelteSet<string>();

  async function closeEndpoint(ep: Endpoint) {
    try {
      await endpointsApi.close(ep.id);
      toasts.success(`Endpoint ${ep.secret_name} closed`);
      await endpointsStore.refresh();
    } catch (err) {
      const msg = isCommandError(err)
        ? err.message
        : err instanceof Error
          ? err.message
          : String(err);
      toasts.error('Could not close endpoint', msg);
    }
  }

  function toggleExpanded(id: string) {
    if (expanded.has(id)) expanded.delete(id);
    else expanded.add(id);
  }

  // The hint shown when a secret has an open endpoint and you reuse it via `cloak endpoint open`.
  function relatedSecret(secretId: string) {
    if (secretsStore.phase.kind !== 'ok') return undefined;
    return secretsStore.phase.items.find((s) => s.id === secretId);
  }
</script>

{#if !vaultUnlocked}
  <Card>
    <div class="text-center text-sm text-zinc-500 dark:text-zinc-400">
      Unlock the vault to view endpoints.
    </div>
  </Card>
{:else if endpointsStore.phase.kind === 'loading'}
  <Card><p class="text-sm text-zinc-500 dark:text-zinc-400">Loading…</p></Card>
{:else if endpointsStore.phase.kind === 'error'}
  <Card>
    <div
      class="rounded-md border border-rose-300 bg-rose-50 p-4 text-sm text-rose-900 dark:border-rose-900 dark:bg-rose-950 dark:text-rose-100"
    >
      <div class="font-medium">{endpointsStore.phase.code}</div>
      <div class="mt-1">{endpointsStore.phase.message}</div>
    </div>
  </Card>
{:else if endpointsStore.phase.items.length === 0}
  <EmptyState
    title="No open endpoints"
    description="Persistent secrets auto-open on unlock. Session secrets open on demand."
  />
{:else}
  <div class="flex flex-col gap-4">
    {#each endpointsStore.phase.items as ep (ep.id)}
      {@const isExpanded = expanded.has(ep.id)}
      {@const secret = relatedSecret(ep.secret_id)}
      {@const materialized = ep.kind === 'materialized'}
      <Card>
        <div class="flex items-start justify-between gap-4">
          <div class="min-w-0 flex-1">
            <div class="flex items-center gap-2">
              <h3 class="font-semibold text-zinc-900 dark:text-zinc-100">{ep.secret_name}</h3>
              <span
                class="rounded-full bg-zinc-100 px-2 py-0.5 text-xs text-zinc-600 dark:bg-zinc-800 dark:text-zinc-400"
              >
                {ep.type}
              </span>
              {#if materialized}
                <span
                  class="rounded-full bg-amber-100 px-2 py-0.5 text-xs text-amber-700 dark:bg-amber-950 dark:text-amber-300"
                >
                  injected
                </span>
              {:else}
                <span
                  class="rounded-full bg-zinc-100 px-2 py-0.5 text-xs text-zinc-600 dark:bg-zinc-800 dark:text-zinc-400"
                >
                  {ep.mode}
                </span>
              {/if}
            </div>
            <div class="mt-1 font-mono text-xs text-zinc-500 dark:text-zinc-400">
              {materialized ? 'injected — no network listener' : ep.local_addr}
              {#if ep.expires_at}
                · expires {timeAgo(ep.expires_at)}
              {/if}
            </div>
            {#if secret?.description}
              <p class="mt-1 text-xs text-zinc-500 dark:text-zinc-400">{secret.description}</p>
            {/if}

            {#if ep.connection_string}
              <div class="mt-3">
                <MaskedString value={ep.connection_string} label="Connection URL copied" />
              </div>
            {/if}

            {#if !materialized}
              <div class="mt-3 flex flex-wrap gap-x-4 gap-y-1 text-xs text-zinc-500 dark:text-zinc-400">
                <span>{ep.stats.connections_open} open</span>
                <span>{ep.stats.connections_total} total</span>
                <span>↓ {formatBytes(ep.stats.bytes_in)}</span>
                <span>↑ {formatBytes(ep.stats.bytes_out)}</span>
                {#if ep.stats.last_activity}
                  <span>last activity {timeAgo(ep.stats.last_activity)}</span>
                {/if}
              </div>
            {/if}

            {#if ep.env_vars && Object.keys(ep.env_vars).length > 0}
              <button
                type="button"
                onclick={() => toggleExpanded(ep.id)}
                class="mt-3 text-xs text-zinc-600 hover:underline dark:text-zinc-400"
              >
                {isExpanded ? 'Hide' : 'Show'} environment variables
              </button>
              {#if isExpanded}
                <div
                  class="mt-2 overflow-hidden rounded-md border border-zinc-200 dark:border-zinc-800"
                >
                  <!-- table-fixed: the value column can't widen the
                       table when a long URL is revealed; it wraps instead. -->
                  <table class="w-full table-fixed text-xs">
                    <tbody class="divide-y divide-zinc-200 dark:divide-zinc-800">
                      {#each Object.entries(ep.env_vars) as [k, v] (k)}
                        <tr>
                          <td
                            class="w-1/3 break-all px-3 py-2 align-top font-mono text-zinc-700 dark:text-zinc-300"
                          >
                            {k}
                          </td>
                          <td class="px-3 py-2 align-top">
                            <MaskedString value={v} label={`${k} copied`} />
                          </td>
                        </tr>
                      {/each}
                    </tbody>
                  </table>
                </div>
              {/if}
            {/if}
          </div>

          <div class="flex shrink-0 flex-col items-end gap-2">
            {#if materialized}
              <Button variant="secondary" onclick={() => navigate('run', ep.secret_name)}>
                Run…
              </Button>
            {:else}
              <CopyButton value={ep.connection_string} sensitive label="Connection URL copied" />
            {/if}
            <Button variant="ghost" onclick={() => closeEndpoint(ep)}>Close</Button>
          </div>
        </div>
      </Card>
    {/each}
  </div>
{/if}
