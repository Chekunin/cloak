<script lang="ts">
  import { secrets as secretsApi } from '$lib/api';
  import { secretsStore } from '$lib/stores/secrets.svelte';
  import { toasts } from '$lib/stores/toasts.svelte';
  import { navigate } from '$lib/router.svelte';
  import Button from '$lib/components/Button.svelte';
  import Card from '$lib/components/Card.svelte';
  import FormField from '$lib/components/FormField.svelte';
  import Input from '$lib/components/Input.svelte';
  import PasswordInput from '$lib/components/PasswordInput.svelte';
  import EndpointConfigSection from './EndpointConfigSection.svelte';
  import {
    buildEndpointConfig,
    defaultEndpointForm,
    extractErrorMessage,
    validateEndpointConfig,
  } from './helpers';

  let form = $state({
    name: '',
    description: '',
    host: '',
    port: '5432',
    user: '',
    database: '',
    tls_mode: 'prefer' as 'disable' | 'prefer' | 'require' | 'verify-ca' | 'verify-full',
    password: '',
    endpoint: defaultEndpointForm(),
  });

  let errors = $state<Record<string, string>>({});
  let submitting = $state(false);
  let topError = $state<string | null>(null);

  function validate(): boolean {
    const e: Record<string, string> = {};
    if (!form.name.trim()) e.name = 'Required.';
    if (!form.host.trim()) e.host = 'Required.';
    if (!form.user.trim()) e.user = 'Required.';
    if (!form.database.trim()) e.database = 'Required.';
    if (!form.password) e.password = 'Required.';
    const port = Number.parseInt(form.port, 10);
    if (!Number.isFinite(port) || port <= 0 || port > 65535) {
      e.port = 'Port must be 1–65535.';
    }
    Object.assign(e, validateEndpointConfig(form.endpoint));
    errors = e;
    return Object.keys(e).length === 0;
  }

  async function onSubmit(ev: SubmitEvent) {
    ev.preventDefault();
    topError = null;
    if (!validate()) return;
    submitting = true;
    try {
      await secretsApi.create({
        name: form.name.trim(),
        type: 'postgres',
        description: form.description.trim() || undefined,
        config: {
          host: form.host.trim(),
          port: Number.parseInt(form.port, 10),
          user: form.user.trim(),
          database: form.database.trim(),
          tls_mode: form.tls_mode,
        },
        secret: { password: form.password },
        endpoint_config: buildEndpointConfig(form.endpoint),
      });
      toasts.success(`Secret "${form.name}" created`);
      await secretsStore.refresh();
      navigate('secrets');
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
        <Input id="name" bind:value={form.name} placeholder="prod-db" autofocus />
      </FormField>
      <FormField id="description" label="Description">
        <Input id="description" bind:value={form.description} placeholder="optional" />
      </FormField>
    </div>

    <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
      <FormField id="host" label="Upstream host" required error={errors.host}>
        <Input id="host" bind:value={form.host} placeholder="db.example.com" />
      </FormField>
      <FormField id="port" label="Upstream port" required error={errors.port}>
        <Input id="port" type="number" bind:value={form.port} />
      </FormField>
    </div>

    <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
      <FormField id="user" label="Upstream user" required error={errors.user}>
        <Input id="user" bind:value={form.user} placeholder="app_user" />
      </FormField>
      <FormField id="database" label="Database" required error={errors.database}>
        <Input id="database" bind:value={form.database} placeholder="app_db" />
      </FormField>
    </div>

    <FormField id="tls_mode" label="TLS mode (upstream)" hint="How Cloak should connect to the database server.">
      <select
        id="tls_mode"
        bind:value={form.tls_mode}
        class="w-full rounded-md border border-zinc-300 bg-white px-3 py-2 text-sm dark:border-zinc-700 dark:bg-zinc-900"
      >
        <option value="disable">disable — plain TCP</option>
        <option value="prefer">prefer — TLS if available</option>
        <option value="require">require — fail if no TLS</option>
        <option value="verify-ca">verify-ca</option>
        <option value="verify-full">verify-full</option>
      </select>
    </FormField>

    <FormField
      id="password"
      label="Upstream password"
      required
      error={errors.password}
      hint="Stored encrypted; never written to disk in plaintext."
    >
      <PasswordInput id="password" bind:value={form.password} autocomplete="new-password" />
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
      <Button variant="ghost" type="button" onclick={() => navigate('secrets')} disabled={submitting}>
        Cancel
      </Button>
      <Button type="submit" loading={submitting} disabled={submitting}>Create secret</Button>
    </div>
  </form>
</Card>
