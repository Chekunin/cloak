<script lang="ts">
  /**
   * Shared `endpoint_config` editor. Used by all four type-specific forms.
   *
   * Binds to a single `config` object so each parent form can spread it into
   * the create request without further mapping.
   */

  import FormField from '$lib/components/FormField.svelte';
  import Input from '$lib/components/Input.svelte';
  import type { EndpointConfig } from '$lib/api';

  interface Props {
    config: EndpointConfig & { persistent_port_str?: string; session_ttl_str?: string };
    errors?: Partial<Record<'persistentPort' | 'sessionTtl', string>>;
  }

  let { config = $bindable(), errors = {} }: Props = $props();
</script>

<div class="flex flex-col gap-4 border-t border-zinc-200 pt-4 dark:border-zinc-800">
  <h3 class="text-sm font-medium uppercase tracking-wider text-zinc-500 dark:text-zinc-400">
    Local endpoint
  </h3>

  <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
    <FormField id="ep-mode" label="Mode">
      <select
        id="ep-mode"
        bind:value={config.mode}
        class="w-full rounded-md border border-zinc-300 bg-white px-3 py-2 text-sm dark:border-zinc-700 dark:bg-zinc-900"
      >
        <option value="persistent">persistent — stays open on unlock</option>
        <option value="session">session — TTL-bounded</option>
      </select>
    </FormField>
    {#if config.mode === 'persistent'}
      <FormField
        id="ep-port"
        label="Persistent port"
        hint="Blank for auto-assignment."
        error={errors.persistentPort}
      >
        <Input id="ep-port" type="number" bind:value={config.persistent_port_str as string} />
      </FormField>
    {:else}
      <FormField
        id="ep-ttl"
        label="Session TTL (seconds)"
        hint="Default 3600."
        error={errors.sessionTtl}
      >
        <Input id="ep-ttl" type="number" bind:value={config.session_ttl_str as string} />
      </FormField>
    {/if}
  </div>

  <label class="flex items-start gap-3 text-sm">
    <input
      type="checkbox"
      bind:checked={config.require_local_auth}
      class="mt-0.5 size-4 accent-zinc-700 dark:accent-zinc-300"
    />
    <span class="text-zinc-700 dark:text-zinc-300">
      <span class="font-medium">Require local authentication</span> — generates a fresh per-endpoint
      password that clients must supply.
    </span>
  </label>
</div>
