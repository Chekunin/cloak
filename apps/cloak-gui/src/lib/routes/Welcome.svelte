<script lang="ts">
  import { vault, isCommandError } from '$lib/api';
  import { navigate } from '$lib/router.svelte';
  import { vaultStore } from '$lib/stores/vault.svelte';
  import { toasts } from '$lib/stores/toasts.svelte';
  import Button from '$lib/components/Button.svelte';
  import Card from '$lib/components/Card.svelte';
  import FormField from '$lib/components/FormField.svelte';
  import PasswordInput from '$lib/components/PasswordInput.svelte';

  let password = $state('');
  let confirm = $state('');
  let acknowledged = $state(false);
  let submitting = $state(false);
  let error = $state<string | null>(null);

  // We block init if the vault is already initialised — show a redirect button instead.
  const alreadyInitialised = $derived(
    vaultStore.phase.kind === 'ok' && vaultStore.phase.status.state !== 'uninitialized',
  );

  async function onSubmit(e: SubmitEvent) {
    e.preventDefault();
    error = null;
    if (password.length < 8) {
      error = 'Password must be at least 8 characters.';
      return;
    }
    if (password !== confirm) {
      error = "Passwords don't match.";
      return;
    }
    if (!acknowledged) {
      error = 'Please acknowledge the no-recovery warning.';
      return;
    }
    submitting = true;
    try {
      await vault.init(password);
      toasts.success('Vault initialised', 'Now unlock it to begin.');
      password = '';
      confirm = '';
      await vaultStore.refresh();
      navigate('unlock');
    } catch (err) {
      if (isCommandError(err)) {
        error = err.hint ? `${err.message} — ${err.hint}` : err.message;
      } else {
        error = err instanceof Error ? err.message : String(err);
      }
    } finally {
      submitting = false;
    }
  }
</script>

<div class="mx-auto flex max-w-xl flex-col gap-6 p-8">
  <header class="text-center">
    <h1 class="text-2xl font-semibold tracking-tight text-zinc-900 dark:text-zinc-100">
      Welcome to Cloak
    </h1>
    <p class="mt-2 text-sm text-zinc-500 dark:text-zinc-400">
      Set the master password that will protect everything you store.
    </p>
  </header>

  {#if alreadyInitialised}
    <Card title="Vault already initialised">
      <p class="text-sm text-zinc-600 dark:text-zinc-400">
        This Cloak instance has a vault. Unlock it to continue.
      </p>
      {#snippet footer()}
        <div class="flex justify-end">
          <Button onclick={() => navigate('unlock')}>Go to unlock</Button>
        </div>
      {/snippet}
    </Card>
  {:else}
    <Card>
      <form onsubmit={onSubmit} class="flex flex-col gap-4">
        <FormField id="pw" label="Master password" required hint="At least 8 characters.">
          <PasswordInput
            id="pw"
            bind:value={password}
            autocomplete="new-password"
            autofocus
            disabled={submitting}
          />
        </FormField>
        <FormField id="confirm" label="Confirm master password" required>
          <PasswordInput
            id="confirm"
            bind:value={confirm}
            autocomplete="new-password"
            disabled={submitting}
          />
        </FormField>

        <label
          class="flex items-start gap-3 rounded-md border border-amber-300 bg-amber-50 p-3 text-sm dark:border-amber-900 dark:bg-amber-950"
        >
          <input
            type="checkbox"
            bind:checked={acknowledged}
            class="mt-0.5 size-4 accent-amber-600"
            disabled={submitting}
          />
          <span class="text-amber-900 dark:text-amber-100">
            I understand that Cloak <strong>cannot reset</strong> this password. If I forget it,
            every stored credential becomes permanently inaccessible.
          </span>
        </label>

        {#if error}
          <div
            class="rounded-md border border-rose-300 bg-rose-50 px-3 py-2 text-sm text-rose-900 dark:border-rose-900 dark:bg-rose-950 dark:text-rose-100"
          >
            {error}
          </div>
        {/if}

        <div class="flex justify-end pt-2">
          <Button type="submit" loading={submitting} disabled={submitting}>Create vault</Button>
        </div>
      </form>
    </Card>
  {/if}
</div>
