<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { endpoints as endpointsApi, isCommandError } from '$lib/api';
  import { endpointsStore } from '$lib/stores/endpoints.svelte';
  import { secretsStore } from '$lib/stores/secrets.svelte';
  import { vaultStore } from '$lib/stores/vault.svelte';
  import { toasts } from '$lib/stores/toasts.svelte';
  import Button from '$lib/components/Button.svelte';
  import Card from '$lib/components/Card.svelte';
  import EndpointList from '$lib/components/EndpointList.svelte';

  const vaultUnlocked = $derived(
    vaultStore.phase.kind === 'ok' && vaultStore.phase.status.state === 'unlocked',
  );

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

  <EndpointList />

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
