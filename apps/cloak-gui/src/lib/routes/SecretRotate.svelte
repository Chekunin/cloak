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
  import PasswordInput from '$lib/components/PasswordInput.svelte';
  import KeyValueList from '$lib/components/KeyValueList.svelte';
  import { extractErrorMessage } from './secret-forms/helpers';

  const secretName = $derived(router.route.params[0] ?? '');

  let rec = $state<Secret | null>(null);
  let loadError = $state<string | null>(null);

  // Per-type form fields. We initialise them blank — rotating means replacing
  // the existing payload entirely; we never pre-populate with the old value
  // (we'd have to decrypt it for that, and the GUI deliberately doesn't).
  let pgPassword = $state('');
  let mysqlPassword = $state('');
  let sshAuthMethod = $state<'password' | 'private_key'>('password');
  let sshPassword = $state('');
  let sshKeyPem = $state('');
  let sshPassphrase = $state('');
  let httpInject = $state<{ key: string; value: string }[]>([
    { key: 'Authorization', value: 'Bearer {{ .api_key }}' },
  ]);
  let httpValues = $state<{ key: string; value: string }[]>([{ key: 'api_key', value: '' }]);

  let submitting = $state(false);
  let topError = $state<string | null>(null);

  // Svelte parses `{ ... }` inside attribute strings as expressions; using a
  // script-level constant avoids the braces ever entering markup.
  const HEADER_PLACEHOLDER = 'Template, e.g. Bearer {{ .api_key }}';

  onMount(async () => {
    if (!secretName) {
      navigate('secrets');
      return;
    }
    try {
      rec = await secretsApi.get(secretName);
    } catch (err) {
      loadError = extractErrorMessage(err);
    }
  });

  function pairsToObject(pairs: { key: string; value: string }[]): Record<string, string> {
    const out: Record<string, string> = {};
    for (const p of pairs) {
      const k = p.key.trim();
      if (!k) continue;
      out[k] = p.value;
    }
    return out;
  }

  function buildSecret(): Record<string, unknown> | null {
    if (!rec) return null;
    switch (rec.type) {
      case 'postgres':
        return { password: pgPassword };
      case 'mysql':
        return { password: mysqlPassword };
      case 'ssh':
        return sshAuthMethod === 'password'
          ? { auth_method: 'password', password: sshPassword }
          : {
              auth_method: 'private_key',
              private_key_pem: sshKeyPem,
              ...(sshPassphrase ? { passphrase: sshPassphrase } : {}),
            };
      case 'http':
        return {
          inject: { headers: pairsToObject(httpInject) },
          values: pairsToObject(httpValues),
        };
    }
  }

  function isValid(): string | null {
    if (!rec) return 'Secret not loaded.';
    switch (rec.type) {
      case 'postgres':
        return pgPassword ? null : 'Password is required.';
      case 'mysql':
        return mysqlPassword ? null : 'Password is required.';
      case 'ssh':
        if (sshAuthMethod === 'password') return sshPassword ? null : 'Password is required.';
        return sshKeyPem.trim() ? null : 'PEM is required.';
      case 'http':
        return null; // empty maps are legal — the user can clear injection rules
    }
  }

  async function onSubmit(ev: SubmitEvent) {
    ev.preventDefault();
    topError = null;
    const err = isValid();
    if (err) {
      topError = err;
      return;
    }
    if (!rec) return;
    submitting = true;
    try {
      const secret = buildSecret();
      if (!secret) {
        topError = 'Unsupported secret type.';
        return;
      }
      await secretsApi.update({
        id_or_name: rec.id,
        secret,
      });
      toasts.success(`Rotated ${rec.name}`, 'Open connections continue to use the old credential.');
      await secretsStore.refresh();
      navigate('secrets');
    } catch (e) {
      topError = isCommandError(e) ? e.message : extractErrorMessage(e);
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
      Rotate secret material
    </h1>
    <p class="text-sm text-zinc-500 dark:text-zinc-400">
      Replaces the encrypted payload. The on-disk ciphertext is overwritten; existing endpoints keep
      working until next decrypt (next connection).
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
    <Card title={rec.name} description="Type: {rec.type}">
      <form onsubmit={onSubmit} class="flex flex-col gap-4">
        {#if rec.type === 'postgres'}
          <FormField id="pg_pw" label="New upstream password" required>
            <PasswordInput id="pg_pw" bind:value={pgPassword} autocomplete="new-password" />
          </FormField>
        {:else if rec.type === 'mysql'}
          <FormField id="my_pw" label="New upstream password" required>
            <PasswordInput id="my_pw" bind:value={mysqlPassword} autocomplete="new-password" />
          </FormField>
        {:else if rec.type === 'ssh'}
          <FormField id="auth_method" label="Authentication method">
            <select
              id="auth_method"
              bind:value={sshAuthMethod}
              class="w-full rounded-md border border-zinc-300 bg-white px-3 py-2 text-sm dark:border-zinc-700 dark:bg-zinc-900"
            >
              <option value="password">password</option>
              <option value="private_key">private_key (PEM)</option>
            </select>
          </FormField>
          {#if sshAuthMethod === 'password'}
            <FormField id="ssh_pw" label="New SSH password" required>
              <PasswordInput id="ssh_pw" bind:value={sshPassword} autocomplete="new-password" />
            </FormField>
          {:else}
            <FormField id="pem" label="New private key PEM" required>
              <textarea
                id="pem"
                bind:value={sshKeyPem}
                rows="6"
                placeholder="-----BEGIN OPENSSH PRIVATE KEY-----&#10;..."
                class="w-full select-text rounded-md border border-zinc-300 bg-white px-3 py-2 font-mono text-xs dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-100"
              ></textarea>
            </FormField>
            <FormField id="passphrase" label="Key passphrase">
              <PasswordInput id="passphrase" bind:value={sshPassphrase} autocomplete="new-password" />
            </FormField>
          {/if}
        {:else if rec.type === 'http'}
          <FormField id="injectHeaders" label="Inject request headers">
            <KeyValueList
              bind:pairs={httpInject}
              keyPlaceholder="Header name"
              valuePlaceholder={HEADER_PLACEHOLDER}
              addLabel="Add header"
            />
          </FormField>
          <FormField id="values" label="Secret values">
            <KeyValueList
              bind:pairs={httpValues}
              keyPlaceholder="Value key"
              valuePlaceholder="The actual secret"
              sensitiveValues
              addLabel="Add value"
            />
          </FormField>
        {/if}

        {#if topError}
          <div
            class="rounded-md border border-rose-300 bg-rose-50 px-3 py-2 text-sm text-rose-900 dark:border-rose-900 dark:bg-rose-950 dark:text-rose-100"
          >
            {topError}
          </div>
        {/if}

        <div class="flex items-center justify-end gap-2 pt-2">
          <Button
            variant="ghost"
            type="button"
            onclick={() => navigate('secrets')}
            disabled={submitting}>Cancel</Button
          >
          <Button type="submit" variant="danger" loading={submitting} disabled={submitting}>
            Rotate secret
          </Button>
        </div>
      </form>
    </Card>
  {/if}
</div>
