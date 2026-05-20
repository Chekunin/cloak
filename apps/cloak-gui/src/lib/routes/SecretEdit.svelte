<script lang="ts">
  import { onMount } from 'svelte';
  import { secrets as secretsApi, isCommandError } from '$lib/api';
  import type { Secret } from '$lib/api';
  import { router, navigate } from '$lib/router.svelte';
  import { secretsStore } from '$lib/stores/secrets.svelte';
  import { toasts } from '$lib/stores/toasts.svelte';
  import Button from '$lib/components/Button.svelte';
  import Card from '$lib/components/Card.svelte';
  import FormField from '$lib/components/FormField.svelte';
  import Input from '$lib/components/Input.svelte';
  import EndpointConfigSection from './secret-forms/EndpointConfigSection.svelte';
  import {
    buildEndpointConfig,
    defaultEndpointForm,
    extractErrorMessage,
    validateEndpointConfig,
  } from './secret-forms/helpers';

  // The route is `secrets:edit:<name>`; the name is the first param.
  const secretName = $derived(router.route.params[0] ?? '');

  let rec = $state<Secret | null>(null);
  let loadError = $state<string | null>(null);

  let form = $state({
    description: '',
    endpoint: defaultEndpointForm(),
  });

  let errors = $state<Record<string, string>>({});
  let submitting = $state(false);
  let topError = $state<string | null>(null);

  onMount(async () => {
    if (!secretName) {
      navigate('secrets');
      return;
    }
    try {
      rec = await secretsApi.get(secretName);
      form.description = rec.description ?? '';
      form.endpoint.mode = rec.endpoint_config.mode ?? 'persistent';
      form.endpoint.persistent_port_str = rec.endpoint_config.persistent_port
        ? String(rec.endpoint_config.persistent_port)
        : '';
      form.endpoint.session_ttl_str = rec.endpoint_config.session_ttl_seconds
        ? String(rec.endpoint_config.session_ttl_seconds)
        : '3600';
      form.endpoint.require_local_auth = rec.endpoint_config.require_local_auth ?? true;
    } catch (err) {
      loadError = extractErrorMessage(err);
    }
  });

  function validate(): boolean {
    const e = validateEndpointConfig(form.endpoint);
    errors = e;
    return Object.keys(e).length === 0;
  }

  async function onSubmit(ev: SubmitEvent) {
    ev.preventDefault();
    topError = null;
    if (!rec) return;
    if (!validate()) return;
    submitting = true;
    try {
      await secretsApi.update({
        id_or_name: rec.id,
        description: form.description,
        endpoint_config: buildEndpointConfig(form.endpoint),
      });
      toasts.success(`Updated ${rec.name}`);
      await secretsStore.refresh();
      navigate('secrets');
    } catch (err) {
      topError = isCommandError(err) ? err.message : extractErrorMessage(err);
    } finally {
      submitting = false;
    }
  }
</script>

<div class="mx-auto flex max-w-2xl flex-col gap-6 p-8">
  <header>
    <button
      type="button"
      class="text-xs text-zinc-500 hover:underline dark:text-zinc-400"
      onclick={() => navigate('secrets')}
    >
      ← Back to secrets
    </button>
    <h1 class="mt-2 text-2xl font-semibold tracking-tight text-zinc-900 dark:text-zinc-100">
      Edit secret
    </h1>
    <p class="text-sm text-zinc-500 dark:text-zinc-400">
      Description and endpoint configuration. Upstream credentials are rotated separately.
    </p>
  </header>

  {#if loadError}
    <Card>
      <div
        class="rounded-md border border-rose-300 bg-rose-50 p-4 text-sm text-rose-900 dark:border-rose-900 dark:bg-rose-950 dark:text-rose-100"
      >
        Could not load secret: {loadError}
      </div>
    </Card>
  {:else if !rec}
    <Card><p class="text-sm text-zinc-500 dark:text-zinc-400">Loading…</p></Card>
  {:else}
    {@const r = rec}
    <Card title={r.name} description={`Type: ${r.type}`}>
      <form onsubmit={onSubmit} class="flex flex-col gap-4">
        <FormField id="description" label="Description">
          <Input id="description" bind:value={form.description} placeholder="optional" />
        </FormField>

        <EndpointConfigSection bind:config={form.endpoint} errors={errors} />

        {#if topError}
          <div
            class="rounded-md border border-rose-300 bg-rose-50 px-3 py-2 text-sm text-rose-900 dark:border-rose-900 dark:bg-rose-950 dark:text-rose-100"
          >
            {topError}
          </div>
        {/if}

        <div class="flex items-center justify-between gap-2 pt-2">
          <button
            type="button"
            class="text-xs text-zinc-500 hover:underline dark:text-zinc-400"
            onclick={() => navigate('secrets:rotate', r.name)}
          >
            Rotate secret material →
          </button>
          <div class="flex items-center gap-2">
            <Button
              variant="ghost"
              type="button"
              onclick={() => navigate('secrets')}
              disabled={submitting}>Cancel</Button
            >
            <Button type="submit" loading={submitting} disabled={submitting}>Save changes</Button>
          </div>
        </div>
      </form>
    </Card>
  {/if}
</div>
