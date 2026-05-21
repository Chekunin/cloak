<script lang="ts">
  /**
   * One secret rendered together with its live endpoint. The endpoint is the
   * secret's *running state* — a secret is either Running (a local listener,
   * or injected env values) or Stopped. Start/Stop toggles that here.
   */

  import { navigate } from '$lib/router.svelte';
  import { endpoints as endpointsApi, isCommandError } from '$lib/api';
  import type { Endpoint, Secret } from '$lib/api';
  import { endpointsStore } from '$lib/stores/endpoints.svelte';
  import { toasts } from '$lib/stores/toasts.svelte';
  import { formatBytes, isMeaningfulTime, timeAgo, timeUntil } from '$lib/format';
  import Button from './Button.svelte';
  import MaskedString from './MaskedString.svelte';
  import CopyButton from './CopyButton.svelte';

  interface Props {
    secret: Secret;
    /** The secret's open endpoint, if one is currently running. */
    endpoint?: Endpoint;
    onEdit: (name: string) => void;
    onDelete: (name: string) => void;
  }

  const { secret, endpoint, onEdit, onDelete }: Props = $props();

  const isEnv = $derived(secret.type === 'env');
  const materialized = $derived(endpoint?.kind === 'materialized');
  const running = $derived(endpoint !== undefined);

  const statusLabel = $derived(
    !running ? 'Stopped' : materialized ? 'Injected' : 'Running',
  );

  let busy = $state(false);
  let showEnv = $state(false);

  function errMessage(err: unknown): string {
    return isCommandError(err)
      ? err.message
      : err instanceof Error
        ? err.message
        : String(err);
  }

  async function startEndpoint() {
    busy = true;
    try {
      await endpointsApi.open(secret.name, 0);
      toasts.success(`Endpoint started for ${secret.name}`);
      await endpointsStore.refresh();
    } catch (err) {
      toasts.error('Could not start endpoint', errMessage(err));
    } finally {
      busy = false;
    }
  }

  async function stopEndpoint() {
    if (!endpoint) return;
    busy = true;
    try {
      await endpointsApi.close(endpoint.id);
      toasts.success(`Endpoint stopped for ${secret.name}`);
      await endpointsStore.refresh();
    } catch (err) {
      toasts.error('Could not stop endpoint', errMessage(err));
    } finally {
      busy = false;
    }
  }
</script>

<div
  class="overflow-hidden rounded-xl border border-zinc-200 bg-white shadow-sm dark:border-zinc-800 dark:bg-zinc-900"
>
  <div class="flex flex-col gap-3 p-5">
    <!-- Header: status + identity -->
    <div class="flex items-start justify-between gap-3">
      <div class="min-w-0">
        <div class="flex flex-wrap items-center gap-2">
          <span
            class="size-2 shrink-0 rounded-full {running
              ? 'bg-emerald-500'
              : 'bg-zinc-300 dark:bg-zinc-600'}"
            aria-hidden="true"
          ></span>
          <h3 class="font-semibold text-zinc-900 dark:text-zinc-100">{secret.name}</h3>
          <span
            class="rounded-full bg-zinc-100 px-2 py-0.5 text-xs text-zinc-600 dark:bg-zinc-800 dark:text-zinc-400"
          >
            {secret.type}
          </span>
          {#if secret.endpoint_config.mode}
            <span
              class="rounded-full bg-zinc-100 px-2 py-0.5 text-xs text-zinc-600 dark:bg-zinc-800 dark:text-zinc-400"
            >
              {secret.endpoint_config.mode}
            </span>
          {/if}
        </div>
        {#if secret.description}
          <p class="mt-1 text-sm text-zinc-500 dark:text-zinc-400">{secret.description}</p>
        {/if}
      </div>
      <div class="flex shrink-0 items-center gap-2">
        <span
          class="rounded-full px-2.5 py-1 text-xs font-medium {running
            ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-950 dark:text-emerald-300'
            : 'bg-zinc-100 text-zinc-500 dark:bg-zinc-800 dark:text-zinc-400'}"
        >
          {statusLabel}
        </span>
        {#if running}
          <Button variant="ghost" onclick={stopEndpoint} loading={busy}>Stop</Button>
        {:else}
          <Button variant="secondary" onclick={startEndpoint} loading={busy}>Start</Button>
        {/if}
      </div>
    </div>

    <!-- Endpoint panel: the secret's running state -->
    <div class="rounded-lg border border-zinc-200 bg-zinc-50 p-3 dark:border-zinc-800 dark:bg-zinc-950/40">
      {#if !endpoint}
        <p class="text-sm text-zinc-500 dark:text-zinc-400">
          {#if isEnv}
            Not running. Start it to inject this secret's variables into commands.
          {:else}
            Not running. Start it to expose a local <code class="font-mono">127.0.0.1</code> endpoint.
          {/if}
        </p>
      {:else}
        <div class="flex items-start justify-between gap-3">
          <div class="min-w-0 flex-1">
            <div class="font-mono text-xs text-zinc-600 dark:text-zinc-300">
              {materialized ? 'Injected — no network listener' : endpoint.local_addr}
              {#if isMeaningfulTime(endpoint.expires_at)}
                <span class="text-zinc-400 dark:text-zinc-500">· expires {timeUntil(endpoint.expires_at)}</span>
              {/if}
            </div>

            {#if endpoint.connection_string}
              <div class="mt-2">
                <MaskedString
                  value={endpoint.connection_string}
                  sensitive={false}
                  label="Connection URL copied"
                />
              </div>
            {/if}

            {#if !materialized}
              <div class="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-zinc-500 dark:text-zinc-400">
                <span>{endpoint.stats.connections_open} open</span>
                <span>{endpoint.stats.connections_total} total</span>
                <span>↓ {formatBytes(endpoint.stats.bytes_in)}</span>
                <span>↑ {formatBytes(endpoint.stats.bytes_out)}</span>
                {#if isMeaningfulTime(endpoint.stats.last_activity)}
                  <span>last activity {timeAgo(endpoint.stats.last_activity)}</span>
                {/if}
              </div>
            {/if}

            {#if endpoint.env_vars && Object.keys(endpoint.env_vars).length > 0}
              <button
                type="button"
                onclick={() => (showEnv = !showEnv)}
                class="mt-2 text-xs text-zinc-600 hover:underline dark:text-zinc-400"
              >
                {showEnv ? 'Hide' : 'Show'} environment variables
              </button>
              {#if showEnv}
                <div class="mt-2 overflow-hidden rounded-md border border-zinc-200 dark:border-zinc-800">
                  <!-- table-fixed: a revealed long value wraps instead of widening the table. -->
                  <table class="w-full table-fixed text-xs">
                    <tbody class="divide-y divide-zinc-200 dark:divide-zinc-800">
                      {#each Object.entries(endpoint.env_vars) as [k, v] (k)}
                        <tr>
                          <td
                            class="w-1/3 break-all bg-white px-3 py-2 align-top font-mono text-zinc-700 dark:bg-zinc-900 dark:text-zinc-300"
                          >
                            {k}
                          </td>
                          <td class="bg-white px-3 py-2 align-top dark:bg-zinc-900">
                            <MaskedString value={v} sensitive={false} label={`${k} copied`} />
                          </td>
                        </tr>
                      {/each}
                    </tbody>
                  </table>
                </div>
              {/if}
            {/if}
          </div>

          {#if materialized || endpoint.connection_string}
            <div class="flex shrink-0 flex-col items-end gap-2">
              {#if materialized}
                <Button variant="secondary" onclick={() => navigate('run', secret.name)}>
                  Run…
                </Button>
              {:else}
                <CopyButton value={endpoint.connection_string} sensitive label="Connection URL copied" />
              {/if}
            </div>
          {/if}
        </div>
      {/if}
    </div>
  </div>

  <!-- Footer: secret-level actions -->
  <div
    class="flex items-center justify-between border-t border-zinc-200 bg-zinc-50/60 px-5 py-2.5 text-xs dark:border-zinc-800 dark:bg-zinc-900/30"
  >
    <div class="flex gap-4">
      <button
        type="button"
        class="text-zinc-600 hover:underline dark:text-zinc-300"
        onclick={() => onEdit(secret.name)}
      >
        Edit
      </button>
      <button
        type="button"
        class="text-zinc-600 hover:underline dark:text-zinc-300"
        onclick={() => navigate('secrets:rotate', secret.name)}
      >
        Rotate
      </button>
    </div>
    <button
      type="button"
      class="text-rose-600 hover:underline dark:text-rose-400"
      onclick={() => onDelete(secret.name)}
    >
      Delete
    </button>
  </div>
</div>
