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
    port: '22',
    user: '',
    host_key_fingerprint: '',
    auth_method: 'password' as 'password' | 'private_key',
    password: '',
    private_key_pem: '',
    passphrase: '',
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
    if (!form.host_key_fingerprint.trim()) {
      e.host_key_fingerprint = 'Required — Cloak refuses to connect without a pinned fingerprint.';
    }
    const port = Number.parseInt(form.port, 10);
    if (!Number.isFinite(port) || port <= 0 || port > 65535) {
      e.port = 'Port must be 1–65535.';
    }
    if (form.auth_method === 'password') {
      if (!form.password) e.password = 'Required.';
    } else if (!form.private_key_pem.trim()) {
      e.private_key_pem = 'Paste your private-key PEM.';
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
      const secret: Record<string, string> =
        form.auth_method === 'password'
          ? { auth_method: 'password', password: form.password }
          : {
              auth_method: 'private_key',
              private_key_pem: form.private_key_pem,
              ...(form.passphrase ? { passphrase: form.passphrase } : {}),
            };

      await secretsApi.create({
        name: form.name.trim(),
        type: 'ssh',
        description: form.description.trim() || undefined,
        config: {
          host: form.host.trim(),
          port: Number.parseInt(form.port, 10),
          user: form.user.trim(),
          host_key_fingerprint: form.host_key_fingerprint.trim(),
        },
        secret,
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
        <Input id="name" bind:value={form.name} placeholder="prod-server" autofocus />
      </FormField>
      <FormField id="description" label="Description">
        <Input id="description" bind:value={form.description} placeholder="optional" />
      </FormField>
    </div>

    <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
      <FormField id="host" label="Upstream host" required error={errors.host}>
        <Input id="host" bind:value={form.host} placeholder="prod.example.com" />
      </FormField>
      <FormField id="port" label="Upstream port" required error={errors.port}>
        <Input id="port" type="number" bind:value={form.port} />
      </FormField>
    </div>

    <FormField id="user" label="Upstream user" required error={errors.user}>
      <Input id="user" bind:value={form.user} placeholder="deploy" />
    </FormField>

    <FormField
      id="fingerprint"
      label="Host key fingerprint"
      required
      error={errors.host_key_fingerprint}
      hint="Get it with: ssh-keyscan -t ed25519 host | ssh-keygen -lf -"
    >
      <Input
        id="fingerprint"
        bind:value={form.host_key_fingerprint}
        placeholder="SHA256:abc123…"
      />
    </FormField>

    <FormField id="auth_method" label="Authentication method">
      <select
        id="auth_method"
        bind:value={form.auth_method}
        class="w-full rounded-md border border-zinc-300 bg-white px-3 py-2 text-sm dark:border-zinc-700 dark:bg-zinc-900"
      >
        <option value="password">password</option>
        <option value="private_key">private_key (PEM)</option>
      </select>
    </FormField>

    {#if form.auth_method === 'password'}
      <FormField id="ssh_password" label="SSH password" required error={errors.password}>
        <PasswordInput
          id="ssh_password"
          bind:value={form.password}
          autocomplete="new-password"
        />
      </FormField>
    {:else}
      <FormField
        id="pem"
        label="Private key PEM"
        required
        error={errors.private_key_pem}
        hint="Paste the contents (-----BEGIN ... PRIVATE KEY----- through -----END...)."
      >
        <textarea
          id="pem"
          bind:value={form.private_key_pem}
          rows="6"
          placeholder="-----BEGIN OPENSSH PRIVATE KEY-----&#10;..."
          class="w-full select-text rounded-md border border-zinc-300 bg-white px-3 py-2 font-mono text-xs text-zinc-900 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-100"
        ></textarea>
      </FormField>
      <FormField id="passphrase" label="Key passphrase" hint="Leave blank if the key is unencrypted.">
        <PasswordInput
          id="passphrase"
          bind:value={form.passphrase}
          autocomplete="new-password"
        />
      </FormField>
    {/if}

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
