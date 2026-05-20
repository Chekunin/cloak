<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { SvelteSet } from 'svelte/reactivity';
  import { endpoints as endpointsApi, isCommandError } from '$lib/api';
  import type { Endpoint } from '$lib/api';
  import { endpointsStore } from '$lib/stores/endpoints.svelte';
  import { secretsStore } from '$lib/stores/secrets.svelte';
  import { vaultStore } from '$lib/stores/vault.svelte';
  import { toasts } from '$lib/stores/toasts.svelte';
  import { formatBytes, timeAgo } from '$lib/format';
  import Button from '$lib/components/Button.svelte';
  import Card from '$lib/components/Card.svelte';
  import EmptyState from '$lib/components/EmptyState.svelte';
  import MaskedString from '$lib/components/MaskedString.svelte';
  import CopyButton from '$lib/components/CopyButton.svelte';

  const vaultUnlocked = $derived(
    vaultStore.phase.kind === 'ok' && vaultStore.phase.status.state === 'unlocked',
  );

  /** Tracks which endpoint cards are expanded to show env vars. */
  const expanded = new SvelteSet<string>();

  onMount(() => {
    if (vaultUnlocked) {
      endpointsStore.start();
      secretsStore.start();
    }
  });
  onDestroy(() => {
    endpointsStore.stop();
    secretsStore.stop();
  });

  $effect(() => {
    if (vaultUnlocked) {
      endpointsStore.start();
      secretsStore.start();
    } else {
      endpointsStore.stop();
      secretsStore.stop();
    }
  });

  /** Secrets that don't currently have an open endpoint — candidates for manual open. */
  const closedSecrets = $derived.by(() => {
    if (secretsStore.phase.kind !== 'ok') return [];
    if (endpointsStore.phase.kind !== 'ok') return secretsStore.phase.items;
    const openSet = new Set(endpointsStore.phase.items.map((e) => e.secret_id));
    return secretsStore.phase.items.filter((s) => !openSet.has(s.id));
  });

  let openingSecret = $state<string | null>(null);
  async function openEndpoint(secretName: string) {
    openingSecret = secretName;
    try {
      await endpointsApi.open(secretName, 0);
      toasts.success(`Endpoint opened for ${secretName}`);
      await endpointsStore.refresh();
    } catch (err) {
      const msg = isCommandError(err)
        ? err.message
        : err instanceof Error
          ? err.message
          : String(err);
      toasts.error(`Could not open endpoint`, msg);
    } finally {
      openingSecret = null;
    }
  }

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

<div class="flex flex-col gap-6 p-8">
  <header>
    <h1 class="text-2xl font-semibold tracking-tight text-zinc-900 dark:text-zinc-100">
      Endpoints
    </h1>
    <p class="text-sm text-zinc-500 dark:text-zinc-400">
      Local listeners proxying to your upstream services.
    </p>
  </header>

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
                <span
                  class="rounded-full bg-zinc-100 px-2 py-0.5 text-xs text-zinc-600 dark:bg-zinc-800 dark:text-zinc-400"
                >
                  {ep.mode}
                </span>
              </div>
              <div class="mt-1 font-mono text-xs text-zinc-500 dark:text-zinc-400">
                {ep.local_addr}
                {#if ep.expires_at}
                  · expires {timeAgo(ep.expires_at)}
                {/if}
              </div>
              {#if secret?.description}
                <p class="mt-1 text-xs text-zinc-500 dark:text-zinc-400">{secret.description}</p>
              {/if}

              <div class="mt-3">
                <MaskedString value={ep.connection_string} label="Connection URL copied" />
              </div>

              <div class="mt-3 flex flex-wrap gap-x-4 gap-y-1 text-xs text-zinc-500 dark:text-zinc-400">
                <span>{ep.stats.connections_open} open</span>
                <span>{ep.stats.connections_total} total</span>
                <span>↓ {formatBytes(ep.stats.bytes_in)}</span>
                <span>↑ {formatBytes(ep.stats.bytes_out)}</span>
                {#if ep.stats.last_activity}
                  <span>last activity {timeAgo(ep.stats.last_activity)}</span>
                {/if}
              </div>

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
              <CopyButton value={ep.connection_string} sensitive label="Connection URL copied" />
              <Button variant="ghost" onclick={() => closeEndpoint(ep)}>Close</Button>
            </div>
          </div>
        </Card>
      {/each}
    </div>
  {/if}

  {#if vaultUnlocked && closedSecrets.length > 0}
    <Card title="Closed secrets" description="Click to open a session endpoint.">
      <div class="flex flex-wrap gap-2">
        {#each closedSecrets as s (s.id)}
          <Button
            variant="secondary"
            onclick={() => openEndpoint(s.name)}
            disabled={openingSecret !== null}
            loading={openingSecret === s.name}
          >
            {s.name}
            <span class="ml-1 text-xs opacity-60">({s.type})</span>
          </Button>
        {/each}
      </div>
    </Card>
  {/if}
</div>
