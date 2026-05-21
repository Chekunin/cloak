<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { vault, isCommandError } from '$lib/api';
  import { vaultStore } from '$lib/stores/vault.svelte';
  import { endpointsStore } from '$lib/stores/endpoints.svelte';
  import { secretsStore } from '$lib/stores/secrets.svelte';
  import { toasts } from '$lib/stores/toasts.svelte';
  import { navigate } from '$lib/router.svelte';
  import { formatDate, formatTimeout } from '$lib/format';
  import Button from '$lib/components/Button.svelte';
  import Card from '$lib/components/Card.svelte';
  import StatTile from '$lib/components/StatTile.svelte';
  import SecretList from '$lib/components/SecretList.svelte';

  const vaultUnlocked = $derived(
    vaultStore.phase.kind === 'ok' && vaultStore.phase.status.state === 'unlocked',
  );

  const hasSecrets = $derived(
    secretsStore.phase.kind === 'ok' && secretsStore.phase.items.length > 0,
  );

  // Keep the secret + endpoint stores polling while this page is mounted so
  // the list below stays live.
  onMount(() => {
    if (vaultUnlocked) {
      secretsStore.start();
      endpointsStore.start();
    }
  });
  onDestroy(() => {
    secretsStore.stop();
    endpointsStore.stop();
  });

  $effect(() => {
    if (vaultUnlocked) {
      secretsStore.start();
      endpointsStore.start();
    } else {
      secretsStore.stop();
      endpointsStore.stop();
    }
  });

  async function onLock() {
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
</script>

<div class="flex flex-col gap-6 p-8">
  <header class="flex items-center justify-between">
    <div>
      <h1 class="text-2xl font-semibold tracking-tight text-zinc-900 dark:text-zinc-100">
        Dashboard
      </h1>
      <p class="text-sm text-zinc-500 dark:text-zinc-400">
        Your secrets and their live endpoints.
      </p>
    </div>
    {#if vaultUnlocked}
      <Button variant="secondary" onclick={onLock}>Lock vault</Button>
    {/if}
  </header>

  <Card title="Vault status">
    {#if vaultStore.phase.kind === 'loading'}
      <p class="text-zinc-500 dark:text-zinc-400">Loading…</p>
    {:else if vaultStore.phase.kind === 'error'}
      <div
        class="rounded-md border border-rose-300 bg-rose-50 p-4 text-sm text-rose-900 dark:border-rose-900 dark:bg-rose-950 dark:text-rose-100"
      >
        <div class="font-medium">{vaultStore.phase.code}</div>
        <div class="mt-1">{vaultStore.phase.message}</div>
        {#if vaultStore.phase.hint}
          <div class="mt-1 text-xs">{vaultStore.phase.hint}</div>
        {/if}
      </div>
    {:else}
      {@const s = vaultStore.phase.status}
      <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <StatTile label="State" value={s.state} />
        <StatTile
          label="Idle timeout"
          value={formatTimeout(s.idle_timeout_sec)}
          hint={s.state === 'unlocked' && s.expires_at
            ? `Auto-lock at ${formatDate(s.expires_at)}`
            : undefined}
        />
      </div>
    {/if}
  </Card>

  <section class="flex flex-col gap-4">
    <div class="flex items-center justify-between gap-3">
      <div>
        <h2 class="text-lg font-semibold tracking-tight text-zinc-900 dark:text-zinc-100">
          Secrets
        </h2>
        <p class="text-sm text-zinc-500 dark:text-zinc-400">
          Stored credentials, each with an optional local endpoint. Running endpoints sort first.
        </p>
      </div>
      {#if vaultUnlocked && hasSecrets}
        <Button onclick={() => navigate('secrets:create')}>Add secret</Button>
      {/if}
    </div>
    <SecretList />
  </section>
</div>
