<script lang="ts">
  /**
   * Master-password-gated secret editor.
   *
   * Opening it prompts for the vault master password — the daemon re-checks it
   * (a client token is not enough) and audit-logs the call — then decrypts the
   * secret and shows every field editable, including the credential itself.
   * Seeing the values and changing them happen in the same form; Save writes
   * the changes back, Cancel discards them.
   *
   * Usage:
   *   let editName = $state<string | null>(null);
   *   <SecretDialog secretName={editName} onClose={() => (editName = null)} />
   */

  import { secrets as secretsApi, isCommandError } from '$lib/api';
  import type { Secret } from '$lib/api';
  import { secretsStore } from '$lib/stores/secrets.svelte';
  import { toasts } from '$lib/stores/toasts.svelte';
  import Button from './Button.svelte';
  import PasswordInput from './PasswordInput.svelte';
  import FormField from './FormField.svelte';
  import Input from './Input.svelte';
  import EndpointConfigSection from '$lib/routes/secret-forms/EndpointConfigSection.svelte';
  import {
    buildEndpointConfig,
    defaultEndpointForm,
    extractErrorMessage,
    fromEditableFields,
    toEditableFields,
    validateEndpointConfig,
    type EditableField,
  } from '$lib/routes/secret-forms/helpers';

  interface Props {
    /** Secret name (or id) to edit. Non-null opens the dialog. */
    secretName: string | null;
    onClose: () => void;
  }

  const { secretName, onClose }: Props = $props();

  type Phase = 'prompt' | 'working' | 'loaded';
  let phase = $state<Phase>('prompt');
  let password = $state('');
  let promptError = $state<string | null>(null);
  let dialogEl: HTMLDivElement | undefined = $state();

  // Decrypted secret, as a flat editable form.
  let rec = $state<Secret | null>(null);
  let description = $state('');
  let endpoint = $state(defaultEndpointForm());
  let configRows = $state<EditableField[]>([]);
  let secretRows = $state<EditableField[]>([]);
  let errors = $state<Record<string, string>>({});
  let submitting = $state(false);
  let topError = $state<string | null>(null);

  let prevFocus: Element | null = null;

  // Reset on open; restore focus on close.
  $effect(() => {
    if (secretName) {
      phase = 'prompt';
      password = '';
      promptError = null;
      rec = null;
      description = '';
      endpoint = defaultEndpointForm();
      configRows = [];
      secretRows = [];
      errors = {};
      topError = null;
      prevFocus = document.activeElement;
      queueMicrotask(() => dialogEl?.querySelector<HTMLInputElement>('input')?.focus());
    } else if (prevFocus instanceof HTMLElement) {
      prevFocus.focus();
      prevFocus = null;
    }
  });

  /** Esc / backdrop dismissal is blocked once loaded so a stray click can't drop edits. */
  const canDismiss = $derived(phase !== 'loaded');

  function close() {
    // Drop decrypted material + password from memory before unmounting.
    phase = 'prompt';
    password = '';
    configRows = [];
    secretRows = [];
    onClose();
  }

  function explain(err: unknown): string {
    if (isCommandError(err)) {
      return err.hint ? `${err.message} — ${err.hint}` : err.message;
    }
    return err instanceof Error ? err.message : String(err);
  }

  async function unlock() {
    if (!secretName || !password || phase === 'working') return;
    phase = 'working';
    promptError = null;
    try {
      // `reveal` carries config + secret; `get` carries description and
      // endpoint config, which reveal does not return.
      const [s, data] = await Promise.all([
        secretsApi.get(secretName),
        secretsApi.reveal(secretName, password),
      ]);
      password = '';
      rec = s;
      description = s.description ?? '';
      endpoint.mode = s.endpoint_config.mode ?? 'persistent';
      endpoint.persistent_port_str = s.endpoint_config.persistent_port
        ? String(s.endpoint_config.persistent_port)
        : '';
      endpoint.session_ttl_str = s.endpoint_config.session_ttl_seconds
        ? String(s.endpoint_config.session_ttl_seconds)
        : '3600';
      endpoint.require_local_auth = s.endpoint_config.require_local_auth ?? true;
      configRows = toEditableFields(data.config);
      secretRows = toEditableFields(data.secret);
      phase = 'loaded';
    } catch (err) {
      password = '';
      phase = 'prompt';
      promptError = explain(err);
    }
  }

  async function save(ev: SubmitEvent) {
    ev.preventDefault();
    if (!rec || submitting) return;
    topError = null;

    const epErrors = validateEndpointConfig(endpoint);
    errors = epErrors;
    if (Object.keys(epErrors).length > 0) return;

    let config: Record<string, unknown>;
    let secret: Record<string, unknown>;
    try {
      config = fromEditableFields(configRows);
      secret = fromEditableFields(secretRows);
    } catch (err) {
      topError = err instanceof Error ? err.message : String(err);
      return;
    }

    submitting = true;
    try {
      await secretsApi.update({
        id_or_name: rec.id,
        description,
        config,
        secret,
        endpoint_config: buildEndpointConfig(endpoint),
      });
      toasts.success(`Updated ${rec.name}`);
      await secretsStore.refresh();
      close();
    } catch (err) {
      topError = isCommandError(err) ? err.message : extractErrorMessage(err);
    } finally {
      submitting = false;
    }
  }

  function onKeyDown(e: KeyboardEvent) {
    if (!secretName) return;
    if (e.key === 'Escape' && canDismiss) {
      e.preventDefault();
      close();
    }
  }

  const selectClass =
    'w-full rounded-md border border-zinc-300 bg-white px-3 py-2 text-sm dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-100';
  const textareaClass =
    'w-full select-text rounded-md border border-zinc-300 bg-white px-3 py-2 font-mono text-xs text-zinc-900 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-100';
</script>

<svelte:window onkeydown={onKeyDown} />

{#snippet fieldRow(field: EditableField, id: string, masked: boolean)}
  <FormField {id} label={field.path}>
    {#if field.kind === 'boolean'}
      <select {id} bind:value={field.value} class={selectClass}>
        <option value="true">true</option>
        <option value="false">false</option>
      </select>
    {:else if field.kind === 'json' || field.multiline}
      <textarea {id} bind:value={field.value} rows="5" spellcheck="false" class={textareaClass}
      ></textarea>
    {:else if masked}
      <PasswordInput {id} bind:value={field.value} autocomplete="off" />
    {:else}
      <Input {id} type={field.kind === 'number' ? 'number' : 'text'} bind:value={field.value} />
    {/if}
  </FormField>
{/snippet}

{#if secretName}
  <div
    class="fixed inset-0 z-50 flex items-center justify-center bg-zinc-950/60 p-4 backdrop-blur-sm"
    role="presentation"
    onclick={(e) => {
      if (e.target === e.currentTarget && canDismiss) close();
    }}
    onkeydown={() => {}}
  >
    <div
      bind:this={dialogEl}
      role="dialog"
      aria-modal="true"
      aria-labelledby="secret-dialog-title"
      class="flex max-h-[88vh] w-full max-w-2xl flex-col rounded-xl border border-zinc-200 bg-white shadow-xl dark:border-zinc-800 dark:bg-zinc-900"
    >
      <div class="px-6 pb-2 pt-6">
        <h2 id="secret-dialog-title" class="text-base font-semibold text-zinc-900 dark:text-zinc-100">
          Edit secret “{secretName}”
        </h2>
        <p class="mt-1 text-sm text-zinc-600 dark:text-zinc-400">
          {#if phase !== 'loaded'}
            Enter the vault master password to edit this secret. This is recorded in the audit log.
          {:else}
            Change any field, including the stored credential, then save.
          {/if}
        </p>
      </div>

      {#if phase !== 'loaded'}
        <div class="px-6 pb-4">
          <label
            for="secret-dialog-password"
            class="text-xs font-medium text-zinc-700 dark:text-zinc-300"
          >
            Master password
          </label>
          <div class="mt-1.5">
            <PasswordInput
              id="secret-dialog-password"
              bind:value={password}
              disabled={phase === 'working'}
              onkeydown={(e) => {
                if (e.key === 'Enter') {
                  e.preventDefault();
                  void unlock();
                }
              }}
            />
          </div>
          {#if promptError}
            <p class="mt-2 text-xs text-rose-600 dark:text-rose-400">{promptError}</p>
          {/if}
        </div>
        <div
          class="flex items-center justify-end gap-2 rounded-b-xl border-t border-zinc-200 bg-zinc-50/60 px-6 py-3 dark:border-zinc-800 dark:bg-zinc-900/40"
        >
          <Button variant="ghost" onclick={close}>Cancel</Button>
          <Button onclick={() => void unlock()} disabled={!password || phase === 'working'}>
            {phase === 'working' ? 'Unlocking…' : 'Unlock'}
          </Button>
        </div>
      {:else if rec}
        {@const r = rec}
        <form onsubmit={save} class="flex min-h-0 flex-col">
          <div class="flex min-h-0 flex-col gap-4 overflow-y-auto px-6 py-4">
            <p class="text-xs text-zinc-500 dark:text-zinc-400">{r.name} · {r.type}</p>

            <FormField id="edit-description" label="Description">
              <Input id="edit-description" bind:value={description} placeholder="optional" />
            </FormField>

            <div class="flex flex-col gap-4 border-t border-zinc-200 pt-4 dark:border-zinc-800">
              <h3 class="text-sm font-medium uppercase tracking-wider text-zinc-500 dark:text-zinc-400">
                Connection settings
              </h3>
              {#if configRows.length === 0}
                <p class="text-sm text-zinc-500 dark:text-zinc-400">No connection settings.</p>
              {:else}
                {#each configRows as field, i (field.path)}
                  {@render fieldRow(field, `cfg-${i}`, false)}
                {/each}
              {/if}
            </div>

            <div class="flex flex-col gap-4 border-t border-zinc-200 pt-4 dark:border-zinc-800">
              <h3 class="text-sm font-medium uppercase tracking-wider text-zinc-500 dark:text-zinc-400">
                Secret material
              </h3>
              {#if secretRows.length === 0}
                <p class="text-sm text-zinc-500 dark:text-zinc-400">No stored credential material.</p>
              {:else}
                {#each secretRows as field, i (field.path)}
                  {@render fieldRow(field, `sec-${i}`, true)}
                {/each}
              {/if}
            </div>

            <EndpointConfigSection bind:config={endpoint} {errors} />

            {#if topError}
              <div
                class="rounded-md border border-rose-300 bg-rose-50 px-3 py-2 text-sm text-rose-900 dark:border-rose-900 dark:bg-rose-950 dark:text-rose-100"
              >
                {topError}
              </div>
            {/if}
          </div>
          <div
            class="flex items-center justify-end gap-2 rounded-b-xl border-t border-zinc-200 bg-zinc-50/60 px-6 py-3 dark:border-zinc-800 dark:bg-zinc-900/40"
          >
            <Button variant="ghost" type="button" onclick={close} disabled={submitting}>Cancel</Button>
            <Button type="submit" loading={submitting} disabled={submitting}>Save</Button>
          </div>
        </form>
      {/if}
    </div>
  </div>
{/if}
