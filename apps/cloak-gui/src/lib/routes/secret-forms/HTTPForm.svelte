<script lang="ts">
  import { secrets as secretsApi } from '$lib/api';
  import { secretsStore } from '$lib/stores/secrets.svelte';
  import { toasts } from '$lib/stores/toasts.svelte';
  import { navigate } from '$lib/router.svelte';
  import Button from '$lib/components/Button.svelte';
  import Card from '$lib/components/Card.svelte';
  import FormField from '$lib/components/FormField.svelte';
  import Input from '$lib/components/Input.svelte';
  import KeyValueList from '$lib/components/KeyValueList.svelte';
  import EndpointConfigSection from './EndpointConfigSection.svelte';
  import {
    buildEndpointConfig,
    defaultEndpointForm,
    extractErrorMessage,
    validateEndpointConfig,
  } from './helpers';

  // Inject rules: header name → template string (with `{{ .key }}` refs).
  // Values: name → secret value (referenced by templates).
  // Strip: list of incoming header names to delete before forwarding.
  let form = $state({
    name: '',
    description: '',
    upstream: '',
    follow_redirects: true,
    injectHeaders: [{ key: 'Authorization', value: 'Bearer {{ .api_key }}' }],
    values: [{ key: 'api_key', value: '' }],
    stripHeaders: [] as { key: string; value: string }[],
    endpoint: defaultEndpointForm(),
  });

  let errors = $state<Record<string, string>>({});
  let submitting = $state(false);
  let topError = $state<string | null>(null);

  // Svelte parses `{ ... }` inside attribute strings as expressions; using a
  // script-level constant avoids the braces ever entering markup.
  const HEADER_PLACEHOLDER = 'Template, e.g. Bearer {{ .api_key }}';

  function validate(): boolean {
    const e: Record<string, string> = {};
    if (!form.name.trim()) e.name = 'Required.';
    if (!form.upstream.trim()) e.upstream = 'Required.';
    try {
      const u = new URL(form.upstream);
      if (u.protocol !== 'http:' && u.protocol !== 'https:') {
        e.upstream = 'Must be a http:// or https:// URL.';
      }
    } catch {
      e.upstream = 'Not a valid URL.';
    }
    Object.assign(e, validateEndpointConfig(form.endpoint));
    errors = e;
    return Object.keys(e).length === 0;
  }

  function pairsToObject(
    pairs: { key: string; value: string }[],
  ): Record<string, string> | undefined {
    const out: Record<string, string> = {};
    for (const p of pairs) {
      const k = p.key.trim();
      if (!k) continue;
      out[k] = p.value;
    }
    return Object.keys(out).length === 0 ? undefined : out;
  }

  async function onSubmit(ev: SubmitEvent) {
    ev.preventDefault();
    topError = null;
    if (!validate()) return;
    submitting = true;
    try {
      const inject: { headers?: Record<string, string> } = {};
      const headers = pairsToObject(form.injectHeaders);
      if (headers) inject.headers = headers;

      const values = pairsToObject(form.values) ?? {};

      const config: Record<string, unknown> = {
        upstream: form.upstream.trim(),
        follow_redirects: form.follow_redirects,
      };
      const strip = form.stripHeaders.map((p) => p.key.trim()).filter(Boolean);
      if (strip.length > 0) config.strip_request_headers = strip;

      await secretsApi.create({
        name: form.name.trim(),
        type: 'http',
        description: form.description.trim() || undefined,
        config,
        secret: { inject, values },
        endpoint_config: buildEndpointConfig(form.endpoint),
      });
      toasts.success(`Secret "${form.name}" created`);
      await secretsStore.refresh();
      navigate('dashboard');
    } catch (err) {
      topError = extractErrorMessage(err);
    } finally {
      submitting = false;
    }
  }
</script>

<Card>
  <form onsubmit={onSubmit} class="flex flex-col gap-4">
    <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
      <FormField id="name" label="Name" required error={errors.name}>
        <Input id="name" bind:value={form.name} placeholder="stripe-api" autofocus />
      </FormField>
      <FormField id="description" label="Description">
        <Input id="description" bind:value={form.description} placeholder="optional" />
      </FormField>
    </div>

    <FormField
      id="upstream"
      label="Upstream URL"
      required
      error={errors.upstream}
      hint="The real service Cloak proxies to."
    >
      <Input
        id="upstream"
        bind:value={form.upstream}
        placeholder="https://api.stripe.com"
      />
    </FormField>

    <label class="flex items-start gap-3 text-sm">
      <input
        type="checkbox"
        bind:checked={form.follow_redirects}
        class="mt-0.5 size-4 accent-zinc-700 dark:accent-zinc-300"
      />
      <span class="text-zinc-700 dark:text-zinc-300">Follow redirects from upstream.</span>
    </label>

    <FormField
      id="injectHeaders"
      label="Inject request headers"
      hint={"Template syntax: {{ .api_key }} references a value below."}
    >
      <KeyValueList
        bind:pairs={form.injectHeaders}
        keyPlaceholder="Header name"
        valuePlaceholder={HEADER_PLACEHOLDER}
        addLabel="Add header"
      />
    </FormField>

    <FormField
      id="values"
      label="Secret values"
      hint="Referenced by the templates above. Encrypted at rest."
    >
      <KeyValueList
        bind:pairs={form.values}
        keyPlaceholder="Value key"
        valuePlaceholder="The actual secret"
        sensitiveValues
        addLabel="Add value"
      />
    </FormField>

    <FormField
      id="stripHeaders"
      label="Strip incoming headers"
      hint="Removed from the client's request before forwarding upstream."
    >
      <KeyValueList
        bind:pairs={form.stripHeaders}
        keyPlaceholder="Header name"
        valueless
        addLabel="Add header to strip"
      />
    </FormField>

    <EndpointConfigSection bind:config={form.endpoint} errors={errors} />

    {#if topError}
      <div
        class="rounded-md border border-rose-300 bg-rose-50 px-3 py-2 text-sm text-rose-900 dark:border-rose-900 dark:bg-rose-950 dark:text-rose-100"
      >
        {topError}
      </div>
    {/if}

    <div class="flex items-center justify-end gap-2 pt-2">
      <Button variant="ghost" type="button" onclick={() => navigate('dashboard')} disabled={submitting}>
        Cancel
      </Button>
      <Button type="submit" loading={submitting} disabled={submitting}>Create secret</Button>
    </div>
  </form>
</Card>
