<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { secrets as secretsApi, isCommandError } from '$lib/api';
  import { secretsStore } from '$lib/stores/secrets.svelte';
  import { vaultStore } from '$lib/stores/vault.svelte';
  import { toasts } from '$lib/stores/toasts.svelte';
  import { navigate } from '$lib/router.svelte';
  import Button from '$lib/components/Button.svelte';
  import Card from '$lib/components/Card.svelte';
  import EmptyState from '$lib/components/EmptyState.svelte';
  import ConfirmDialog, { type ConfirmConfig } from '$lib/components/ConfirmDialog.svelte';

  const vaultUnlocked = $derived(
    vaultStore.phase.kind === 'ok' && vaultStore.phase.status.state === 'unlocked',
  );

  let confirmConfig = $state<ConfirmConfig | null>(null);
  let pendingDelete = $state<string | null>(null);

  function askDelete(name: string) {
    pendingDelete = name;
    confirmConfig = {
      title: `Delete secret "${name}"?`,
      message: 'Any open endpoint will be closed first. The encrypted material is removed from the vault.',
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

  onMount(() => {
    if (vaultUnlocked) secretsStore.start();
  });
  onDestroy(() => secretsStore.stop());

  // Restart polling when the vault transitions to unlocked.
  $effect(() => {
    if (vaultUnlocked) {
      secretsStore.start();
    } else {
      secretsStore.stop();
    }
  });

  function formatDate(iso: string): string {
    const t = new Date(iso);
    return Number.isNaN(t.getTime()) ? iso : t.toLocaleDateString();
  }
</script>

<div class="flex flex-col gap-6 p-8">
  <header class="flex items-center justify-between">
    <div>
      <h1 class="text-2xl font-semibold tracking-tight text-zinc-900 dark:text-zinc-100">
        Secrets
      </h1>
      <p class="text-sm text-zinc-500 dark:text-zinc-400">
        Stored credentials, encrypted at rest with the vault DEK.
      </p>
    </div>
    {#if vaultUnlocked}
      <Button onclick={() => navigate('secrets:create')}>Add secret</Button>
    {/if}
  </header>

  {#if !vaultUnlocked}
    <Card>
      <div class="text-center text-sm text-zinc-500 dark:text-zinc-400">
        Unlock the vault to view secrets.
      </div>
    </Card>
  {:else if secretsStore.phase.kind === 'loading'}
    <Card>
      <p class="text-sm text-zinc-500 dark:text-zinc-400">Loading…</p>
    </Card>
  {:else if secretsStore.phase.kind === 'error'}
    <Card>
      <div
        class="rounded-md border border-rose-300 bg-rose-50 p-4 text-sm text-rose-900 dark:border-rose-900 dark:bg-rose-950 dark:text-rose-100"
      >
        <div class="font-medium">{secretsStore.phase.code}</div>
        <div class="mt-1">{secretsStore.phase.message}</div>
      </div>
    </Card>
  {:else if secretsStore.phase.items.length === 0}
    <EmptyState
      title="No secrets yet"
      description="A secret is a stored credential Cloak proxies for. Add your first one to get a local endpoint."
    >
      {#snippet action()}
        <Button onclick={() => navigate('secrets:create')}>Add your first secret</Button>
      {/snippet}
    </EmptyState>
  {:else}
    <Card>
      <div class="overflow-x-auto">
        <table class="w-full text-sm">
          <thead class="text-left text-xs uppercase tracking-wider text-zinc-500 dark:text-zinc-400">
            <tr class="border-b border-zinc-200 dark:border-zinc-800">
              <th class="px-2 py-2 font-medium">Name</th>
              <th class="px-2 py-2 font-medium">Type</th>
              <th class="px-2 py-2 font-medium">Mode</th>
              <th class="px-2 py-2 font-medium">Port</th>
              <th class="px-2 py-2 font-medium">Created</th>
              <th class="px-2 py-2 font-medium" aria-label="Actions"></th>
            </tr>
          </thead>
          <tbody class="divide-y divide-zinc-200 dark:divide-zinc-800">
            {#each secretsStore.phase.items as item (item.id)}
              <tr>
                <td class="px-2 py-3 font-medium text-zinc-900 dark:text-zinc-100">{item.name}</td>
                <td class="px-2 py-3 text-zinc-600 dark:text-zinc-400">{item.type}</td>
                <td class="px-2 py-3 text-zinc-600 dark:text-zinc-400">
                  {item.endpoint_config.mode ?? '—'}
                </td>
                <td class="px-2 py-3 font-mono text-zinc-600 dark:text-zinc-400">
                  {item.endpoint_config.persistent_port || '—'}
                </td>
                <td class="px-2 py-3 text-zinc-600 dark:text-zinc-400">{formatDate(item.created_at)}</td>
                <td class="px-2 py-3 text-right">
                  <div class="flex justify-end gap-3 text-xs">
                    <button
                      type="button"
                      class="text-zinc-600 hover:underline dark:text-zinc-300"
                      onclick={() => navigate('secrets:edit', item.name)}
                    >
                      Edit
                    </button>
                    <button
                      type="button"
                      class="text-zinc-600 hover:underline dark:text-zinc-300"
                      onclick={() => navigate('secrets:rotate', item.name)}
                    >
                      Rotate
                    </button>
                    <button
                      type="button"
                      class="text-rose-600 hover:underline dark:text-rose-400"
                      onclick={() => askDelete(item.name)}
                    >
                      Delete
                    </button>
                  </div>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    </Card>
  {/if}
</div>

<ConfirmDialog config={confirmConfig} onClose={onConfirmClose} />
