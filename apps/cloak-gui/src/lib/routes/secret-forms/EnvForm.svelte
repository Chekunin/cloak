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
  import { extractErrorMessage } from './helpers';

  // An env secret is a bag of key/value pairs. `inject_env` controls whether
  // they become environment variables; `renderFile` optionally also writes
  // them into a templated credentials file.
  let form = $state({
    name: '',
    description: '',
    values: [{ key: 'AWS_ACCESS_KEY_ID', value: '' }],
    inject_env: true,
    renderFile: false,
    fileBasename: 'credentials',
    filePathEnv: 'AWS_SHARED_CREDENTIALS_FILE',
    fileTemplate:
      '[default]\naws_access_key_id={{ .AWS_ACCESS_KEY_ID }}\naws_secret_access_key={{ .AWS_SECRET_ACCESS_KEY }}\n',
    session_ttl_str: '3600',
  });

  let errors = $state<Record<string, string>>({});
  let submitting = $state(false);
  let topError = $state<string | null>(null);

  const ENV_NAME = /^[A-Za-z_][A-Za-z0-9_]*$/;

  function validate(): boolean {
    const e: Record<string, string> = {};
    if (!form.name.trim()) e.name = 'Required.';

    const keys = form.values.map((p) => p.key.trim()).filter(Boolean);
    if (keys.length === 0) {
      e.values = 'Add at least one key/value pair.';
    } else {
      const bad = keys.find((k) => !ENV_NAME.test(k));
      if (bad) e.values = `"${bad}" is not a valid environment variable name.`;
    }

    if (!form.inject_env && !form.renderFile) {
      e.inject_env = 'The secret must deliver something — inject variables or render a file.';
    }

    if (form.renderFile) {
      if (!form.fileBasename.trim() || form.fileBasename.includes('/')) {
        e.fileBasename = 'A single file name, no slashes.';
      }
      if (!ENV_NAME.test(form.filePathEnv.trim())) {
        e.filePathEnv = 'Not a valid environment variable name.';
      }
      if (!form.fileTemplate.trim()) e.fileTemplate = 'Required.';
    }

    const ttl = Number.parseInt(form.session_ttl_str, 10);
    if (!Number.isFinite(ttl) || ttl <= 0) e.ttl = 'TTL must be a positive integer.';

    errors = e;
    return Object.keys(e).length === 0;
  }

  async function onSubmit(ev: SubmitEvent) {
    ev.preventDefault();
    topError = null;
    if (!validate()) return;
    submitting = true;
    try {
      const values: Record<string, string> = {};
      for (const p of form.values) {
        const k = p.key.trim();
        if (k) values[k] = p.value;
      }

      const config: Record<string, unknown> = { inject_env: form.inject_env };
      if (form.renderFile) {
        config.files = [
          {
            basename: form.fileBasename.trim(),
            path_env: form.filePathEnv.trim(),
            template: form.fileTemplate,
          },
        ];
      }

      await secretsApi.create({
        name: form.name.trim(),
        type: 'env',
        description: form.description.trim() || undefined,
        config,
        secret: { values },
        endpoint_config: {
          mode: 'session',
          session_ttl_seconds: Number.parseInt(form.session_ttl_str, 10),
        },
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
    <div
      class="rounded-md border border-amber-300 bg-amber-50 px-3 py-2 text-xs text-amber-900 dark:border-amber-900 dark:bg-amber-950 dark:text-amber-100"
    >
      An <strong>env</strong> secret is injected into the process you run — its values
      <em>do</em> reach that process. This is the weaker of Cloak's two tiers; prefer a
      proxied type (Postgres, MySQL, SSH, HTTP) when one fits.
    </div>

    <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
      <FormField id="name" label="Name" required error={errors.name}>
        <Input id="name" bind:value={form.name} placeholder="aws-prod" autofocus />
      </FormField>
      <FormField id="description" label="Description">
        <Input id="description" bind:value={form.description} placeholder="optional" />
      </FormField>
    </div>

    <FormField
      id="values"
      label="Key / value pairs"
      required
      error={errors.values}
      hint="Each key is used verbatim as an environment variable name. Encrypted at rest."
    >
      <KeyValueList
        bind:pairs={form.values}
        keyPlaceholder="AWS_SECRET_ACCESS_KEY"
        valuePlaceholder="The actual value"
        sensitiveValues
        addLabel="Add pair"
      />
    </FormField>

    <label class="flex items-start gap-3 text-sm">
      <input
        type="checkbox"
        bind:checked={form.inject_env}
        class="mt-0.5 size-4 accent-zinc-700 dark:accent-zinc-300"
      />
      <span class="text-zinc-700 dark:text-zinc-300">
        <span class="font-medium">Inject as environment variables</span> — turn off if the tool reads
        only a file.
      </span>
    </label>
    {#if errors.inject_env}
      <p class="-mt-2 text-xs text-rose-600 dark:text-rose-400">{errors.inject_env}</p>
    {/if}

    <label class="flex items-start gap-3 text-sm">
      <input
        type="checkbox"
        bind:checked={form.renderFile}
        class="mt-0.5 size-4 accent-zinc-700 dark:accent-zinc-300"
      />
      <span class="text-zinc-700 dark:text-zinc-300">
        <span class="font-medium">Render a credentials file</span> — written 0600 while in use,
        shredded afterwards.
      </span>
    </label>

    {#if form.renderFile}
      <div class="flex flex-col gap-4 rounded-md border border-zinc-200 p-4 dark:border-zinc-800">
        <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <FormField id="file-basename" label="File name" error={errors.fileBasename}>
            <Input id="file-basename" bind:value={form.fileBasename} placeholder="credentials" />
          </FormField>
          <FormField
            id="file-path-env"
            label="Path variable"
            hint="Receives the file's absolute path."
            error={errors.filePathEnv}
          >
            <Input
              id="file-path-env"
              bind:value={form.filePathEnv}
              placeholder="AWS_SHARED_CREDENTIALS_FILE"
            />
          </FormField>
        </div>
        <FormField
          id="file-template"
          label="Template"
          hint={'Go text/template. Reference values as {{ .KEY }}.'}
          error={errors.fileTemplate}
        >
          <textarea
            id="file-template"
            bind:value={form.fileTemplate}
            rows="5"
            spellcheck="false"
            class="w-full rounded-md border border-zinc-300 bg-white px-3 py-2 font-mono text-xs dark:border-zinc-700 dark:bg-zinc-900"
          ></textarea>
        </FormField>
      </div>
    {/if}

    <div class="flex flex-col gap-4 border-t border-zinc-200 pt-4 dark:border-zinc-800">
      <h3 class="text-sm font-medium uppercase tracking-wider text-zinc-500 dark:text-zinc-400">
        Lifetime
      </h3>
      <FormField
        id="ttl"
        label="Session TTL (seconds)"
        hint="An env secret is always session-scoped; the materialization expires after this."
        error={errors.ttl}
      >
        <Input id="ttl" type="number" bind:value={form.session_ttl_str} />
      </FormField>
    </div>

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
