<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { tokens as tokensApi, isCommandError } from '$lib/api';
  import type { Token, TokenInfo } from '$lib/api';
  import { connection } from '$lib/stores/connection.svelte';
  import { toasts } from '$lib/stores/toasts.svelte';
  import { formatDate } from '$lib/format';
  import Button from '$lib/components/Button.svelte';
  import Card from '$lib/components/Card.svelte';
  import EmptyState from '$lib/components/EmptyState.svelte';
  import FormField from '$lib/components/FormField.svelte';
  import Input from '$lib/components/Input.svelte';
  import MaskedString from '$lib/components/MaskedString.svelte';
  import GuiTokenSetup from '$lib/components/GuiTokenSetup.svelte';
  import ConfirmDialog, { type ConfirmConfig } from '$lib/components/ConfirmDialog.svelte';

  /** Authenticated state of *this* GUI process — gates the management UI. */
  const isAuthorised = $derived(
    connection.state.kind === 'connected' && connection.state.hasToken,
  );

  let list = $state<Token[]>([]);
  let errorMessage = $state<string | null>(null);
  let loading = $state(true);

  let newName = $state('');
  let persistInGui = $state(false);
  let creating = $state(false);

  /** The most recently created token, displayed once. Cleared on dismiss. */
  let freshToken = $state<TokenInfo | null>(null);

  let timer: number | null = null;

  onMount(() => {
    if (isAuthorised) {
      void refresh();
      timer = window.setInterval(() => void refresh(), 4000);
    }
  });

  // Start/stop polling when authorisation flips.
  $effect(() => {
    if (isAuthorised && timer === null) {
      void refresh();
      timer = window.setInterval(() => void refresh(), 4000);
    } else if (!isAuthorised && timer !== null) {
      window.clearInterval(timer);
      timer = null;
    }
  });
  onDestroy(() => {
    if (timer !== null) window.clearInterval(timer);
  });

  async function refresh() {
    try {
      list = await tokensApi.list();
      errorMessage = null;
    } catch (err) {
      errorMessage = isCommandError(err)
        ? err.message
        : err instanceof Error
          ? err.message
          : String(err);
    } finally {
      loading = false;
    }
  }

  async function onCreate(e: SubmitEvent) {
    e.preventDefault();
    if (!newName.trim()) return;
    creating = true;
    try {
      freshToken = await tokensApi.create(newName.trim(), persistInGui);
      newName = '';
      persistInGui = false;
      toasts.success('Token created', 'Copy the token now — it is shown only once.');
      await refresh();
    } catch (err) {
      const msg = isCommandError(err)
        ? err.message
        : err instanceof Error
          ? err.message
          : String(err);
      toasts.error('Could not create token', msg);
    } finally {
      creating = false;
    }
  }

  let confirmConfig = $state<ConfirmConfig | null>(null);
  let pendingRevokeId = $state<string | null>(null);

  function askRevoke(t: Token) {
    pendingRevokeId = t.id;
    confirmConfig = {
      title: `Revoke "${t.name}"?`,
      message: 'Any client still using this token will get an "unauthorized" error on its next call.',
      hint: 'This cannot be undone — but you can always issue a new token.',
      variant: 'danger',
      confirmLabel: 'Revoke token',
    };
  }

  async function onConfirmClose(confirmed: boolean) {
    const id = pendingRevokeId;
    confirmConfig = null;
    pendingRevokeId = null;
    if (!confirmed || !id) return;
    try {
      await tokensApi.revoke(id);
      toasts.success('Token revoked');
      await refresh();
    } catch (err) {
      const msg = isCommandError(err)
        ? err.message
        : err instanceof Error
          ? err.message
          : String(err);
      toasts.error('Could not revoke token', msg);
    }
  }
</script>

<ConfirmDialog config={confirmConfig} onClose={onConfirmClose} />

<div class="flex flex-col gap-6 p-8">
  <header>
    <h1 class="text-2xl font-semibold tracking-tight text-zinc-900 dark:text-zinc-100">
      Tokens
    </h1>
    <p class="text-sm text-zinc-500 dark:text-zinc-400">
      Bearer credentials clients send over IPC to authenticate to the daemon.
    </p>
  </header>

  {#if !isAuthorised}
    <GuiTokenSetup />
  {/if}

  {#if freshToken}
    <Card title="New token: {freshToken.name}" description="This value will not be shown again.">
      <MaskedString value={freshToken.token} sensitive label="Token copied" />
      {#snippet footer()}
        <div class="flex justify-end">
          <Button variant="ghost" onclick={() => (freshToken = null)}>Done</Button>
        </div>
      {/snippet}
    </Card>
  {/if}

  {#if isAuthorised}
  <Card title="Issue a new token">
    <form onsubmit={onCreate} class="flex flex-col gap-4">
      <FormField id="tokenName" label="Name" hint="Identifies the consumer in audit logs.">
        <Input
          id="tokenName"
          bind:value={newName}
          placeholder="my-laptop-cursor"
          disabled={creating}
        />
      </FormField>
      <label class="flex items-start gap-3 text-sm">
        <input
          type="checkbox"
          bind:checked={persistInGui}
          class="mt-0.5 size-4 accent-zinc-700 dark:accent-zinc-300"
          disabled={creating}
        />
        <span class="text-zinc-700 dark:text-zinc-300">
          <span class="font-medium">Use as the GUI's own token</span> — subsequent commands from
          this app authenticate with it automatically.
        </span>
      </label>
      <div class="flex justify-end">
        <Button type="submit" loading={creating} disabled={creating || !newName.trim()}>
          Create token
        </Button>
      </div>
    </form>
  </Card>

  <Card title="Existing tokens">
    {#if loading && list.length === 0}
      <p class="text-sm text-zinc-500 dark:text-zinc-400">Loading…</p>
    {:else if errorMessage}
      <div
        class="rounded-md border border-rose-300 bg-rose-50 p-4 text-sm text-rose-900 dark:border-rose-900 dark:bg-rose-950 dark:text-rose-100"
      >
        {errorMessage}
      </div>
    {:else if list.length === 0}
      <EmptyState
        title="No tokens yet"
        description="Tokens authenticate the CLI, this GUI, and any future MCP / GUI client to the daemon."
      />
    {:else}
      <div class="overflow-x-auto">
        <table class="w-full text-sm">
          <thead
            class="text-left text-xs uppercase tracking-wider text-zinc-500 dark:text-zinc-400"
          >
            <tr class="border-b border-zinc-200 dark:border-zinc-800">
              <th class="px-2 py-2 font-medium">Name</th>
              <th class="px-2 py-2 font-medium">Created</th>
              <th class="px-2 py-2 font-medium">Last seen</th>
              <th class="px-2 py-2 font-medium">Status</th>
              <th class="px-2 py-2 font-medium" aria-label="Actions"></th>
            </tr>
          </thead>
          <tbody class="divide-y divide-zinc-200 dark:divide-zinc-800">
            {#each list as t (t.id)}
              <tr>
                <td class="px-2 py-3">
                  <div class="font-medium text-zinc-900 dark:text-zinc-100">{t.name}</div>
                  <div class="font-mono text-xs text-zinc-400 dark:text-zinc-500">{t.id}</div>
                </td>
                <td class="px-2 py-3 text-zinc-600 dark:text-zinc-400">
                  {formatDate(t.created_at)}
                </td>
                <td class="px-2 py-3 text-zinc-600 dark:text-zinc-400">
                  {t.last_seen_at ? formatDate(t.last_seen_at) : '—'}
                </td>
                <td class="px-2 py-3">
                  {#if t.revoked}
                    <span
                      class="rounded-full bg-rose-100 px-2 py-0.5 text-xs text-rose-800 dark:bg-rose-950 dark:text-rose-200"
                    >
                      revoked
                    </span>
                  {:else}
                    <span
                      class="rounded-full bg-emerald-100 px-2 py-0.5 text-xs text-emerald-800 dark:bg-emerald-950 dark:text-emerald-200"
                    >
                      active
                    </span>
                  {/if}
                </td>
                <td class="px-2 py-3 text-right">
                  {#if !t.revoked}
                    <button
                      type="button"
                      class="text-xs text-rose-600 hover:underline dark:text-rose-400"
                      onclick={() => askRevoke(t)}
                    >
                      Revoke
                    </button>
                  {/if}
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </Card>
  {/if}
</div>
