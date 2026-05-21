<script lang="ts">
  /**
   * The unified secrets + endpoints view. Each secret is a card carrying its
   * own endpoint (running) state. Running secrets are sorted to the top, and
   * the search box filters by name.
   */

  import { secrets as secretsApi, isCommandError } from '$lib/api';
  import type { Endpoint } from '$lib/api';
  import { secretsStore } from '$lib/stores/secrets.svelte';
  import { endpointsStore } from '$lib/stores/endpoints.svelte';
  import { vaultStore } from '$lib/stores/vault.svelte';
  import { toasts } from '$lib/stores/toasts.svelte';
  import { navigate } from '$lib/router.svelte';
  import Button from './Button.svelte';
  import Card from './Card.svelte';
  import Input from './Input.svelte';
  import EmptyState from './EmptyState.svelte';
  import SecretCard from './SecretCard.svelte';
  import ConfirmDialog, { type ConfirmConfig } from './ConfirmDialog.svelte';
  import SecretDialog from './SecretDialog.svelte';

  const vaultUnlocked = $derived(
    vaultStore.phase.kind === 'ok' && vaultStore.phase.status.state === 'unlocked',
  );

  let query = $state('');

  /** secret_id → its open endpoint, for joining the two stores. */
  const endpointBySecret = $derived.by(() => {
    const map = new Map<string, Endpoint>();
    if (endpointsStore.phase.kind === 'ok') {
      for (const ep of endpointsStore.phase.items) map.set(ep.secret_id, ep);
    }
    return map;
  });

  /** Secrets filtered by the search box, running ones first, then name A→Z. */
  const rows = $derived.by(() => {
    if (secretsStore.phase.kind !== 'ok') return [];
    const q = query.trim().toLowerCase();
    return secretsStore.phase.items
      .filter((s) => !q || s.name.toLowerCase().includes(q))
      .map((s) => ({ secret: s, endpoint: endpointBySecret.get(s.id) }))
      .sort((a, b) => {
        const rank = (a.endpoint ? 0 : 1) - (b.endpoint ? 0 : 1);
        return rank !== 0 ? rank : a.secret.name.localeCompare(b.secret.name);
      });
  });

  const hasSecrets = $derived(
    secretsStore.phase.kind === 'ok' && secretsStore.phase.items.length > 0,
  );

  // --- delete / reveal dialogs --------------------------------------------

  let confirmConfig = $state<ConfirmConfig | null>(null);
  let pendingDelete = $state<string | null>(null);
  let editName = $state<string | null>(null);

  function askDelete(name: string) {
    pendingDelete = name;
    confirmConfig = {
      title: `Delete secret "${name}"?`,
      message:
        'Any open endpoint will be closed first. The encrypted material is removed from the vault.',
      hint: 'This cannot be undone.',
      variant: 'danger',
      confirmLabel: 'Delete secret',
      requireTyping: name,
    };
  }

  async function onConfirmClose(confirmed: boolean) {
    const name = pendingDelete;
    confirmConfig = null;
    pendingDelete = null;
    if (!confirmed || !name) return;
    try {
      await secretsApi.remove(name);
      toasts.success(`Deleted ${name}`);
      await secretsStore.refresh();
    } catch (err) {
      const msg = isCommandError(err)
        ? err.message
        : err instanceof Error
          ? err.message
          : String(err);
      toasts.error(`Could not delete ${name}`, msg);
    }
  }
</script>

{#if !vaultUnlocked}
  <Card>
    <div class="text-center text-sm text-zinc-500 dark:text-zinc-400">
      Unlock the vault to view secrets.
    </div>
  </Card>
{:else if secretsStore.phase.kind === 'loading'}
  <Card><p class="text-sm text-zinc-500 dark:text-zinc-400">Loading…</p></Card>
{:else if secretsStore.phase.kind === 'error'}
  <Card>
    <div
      class="rounded-md border border-rose-300 bg-rose-50 p-4 text-sm text-rose-900 dark:border-rose-900 dark:bg-rose-950 dark:text-rose-100"
    >
      <div class="font-medium">{secretsStore.phase.code}</div>
      <div class="mt-1">{secretsStore.phase.message}</div>
    </div>
  </Card>
{:else if !hasSecrets}
  <EmptyState
    title="No secrets yet"
    description="A secret is a stored credential Cloak proxies for. Add your first one to get a local endpoint."
  >
    {#snippet action()}
      <Button onclick={() => navigate('secrets:create')}>Add your first secret</Button>
    {/snippet}
  </EmptyState>
{:else}
  <Input bind:value={query} placeholder="Search secrets by name…" />

  {#if rows.length === 0}
    <Card>
      <p class="text-center text-sm text-zinc-500 dark:text-zinc-400">
        No secrets match “{query}”.
      </p>
    </Card>
  {:else}
    <div class="flex flex-col gap-4">
      {#each rows as row (row.secret.id)}
        <SecretCard
          secret={row.secret}
          endpoint={row.endpoint}
          onEdit={(name) => (editName = name)}
          onDelete={askDelete}
        />
      {/each}
    </div>
  {/if}
{/if}

<ConfirmDialog config={confirmConfig} onClose={onConfirmClose} />
<SecretDialog secretName={editName} onClose={() => (editName = null)} />
