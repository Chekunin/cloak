<script lang="ts">
  import { vault, isCommandError } from '$lib/api';
  import { vaultStore } from '$lib/stores/vault.svelte';
  import { toasts } from '$lib/stores/toasts.svelte';
  import { navigate } from '$lib/router.svelte';
  import Button from '$lib/components/Button.svelte';
  import Card from '$lib/components/Card.svelte';
  import StatTile from '$lib/components/StatTile.svelte';

  function formatExpiresAt(iso: string | null | undefined): string {
    if (!iso) return '—';
    const t = new Date(iso);
    if (Number.isNaN(t.getTime())) return iso;
    return t.toLocaleString();
  }

  function formatTimeout(seconds: number): string {
    if (seconds >= 3600) return `${Math.round(seconds / 3600)}h`;
    if (seconds >= 60) return `${Math.round(seconds / 60)}m`;
    return `${seconds}s`;
  }

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
        Vault status and live endpoint activity.
      </p>
    </div>
    {#if vaultStore.phase.kind === 'ok' && vaultStore.phase.status.state === 'unlocked'}
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
      <div class="grid grid-cols-1 gap-3 sm:grid-cols-3">
        <StatTile label="State" value={s.state} />
        <StatTile label="Idle timeout" value={formatTimeout(s.idle_timeout_sec)} />
        <StatTile
          label="Open endpoints"
          value={s.endpoints_open}
          hint={s.state === 'unlocked' && s.expires_at
            ? `Auto-lock at ${formatExpiresAt(s.expires_at)}`
            : undefined}
        />
      </div>
    {/if}
  </Card>
</div>
