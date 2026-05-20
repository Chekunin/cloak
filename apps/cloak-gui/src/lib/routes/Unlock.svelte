<script lang="ts">
  import { vault, isCommandError } from '$lib/api';
  import { navigate } from '$lib/router.svelte';
  import { vaultStore } from '$lib/stores/vault.svelte';
  import { connection } from '$lib/stores/connection.svelte';
  import { toasts } from '$lib/stores/toasts.svelte';
  import Button from '$lib/components/Button.svelte';
  import Card from '$lib/components/Card.svelte';
  import FormField from '$lib/components/FormField.svelte';
  import PasswordInput from '$lib/components/PasswordInput.svelte';

  let password = $state('');
  let submitting = $state(false);
  let error = $state<string | null>(null);

  async function onSubmit(e: SubmitEvent) {
    e.preventDefault();
    error = null;
    if (!password) {
      error = 'Password is required.';
      return;
    }
    submitting = true;
    try {
      await vault.unlock(password);
      toasts.success('Vault unlocked');
      password = '';
      // Pull both stores so the dashboard immediately reflects the new state,
      // including the auto-bootstrapped token (if any) the Rust side just set.
      await Promise.all([vaultStore.refresh(), connection.refresh()]);
      navigate('dashboard');
    } catch (err) {
      if (isCommandError(err)) {
        error = err.message;
      } else {
        error = err instanceof Error ? err.message : String(err);
      }
    } finally {
      submitting = false;
    }
  }
</script>

<div class="mx-auto flex max-w-md flex-col gap-6 p-8">
  <header class="text-center">
    <h1 class="text-2xl font-semibold tracking-tight text-zinc-900 dark:text-zinc-100">
      Unlock vault
    </h1>
    <p class="mt-2 text-sm text-zinc-500 dark:text-zinc-400">
      Enter your master password to decrypt your stored credentials.
    </p>
  </header>

  <Card>
    <form onsubmit={onSubmit} class="flex flex-col gap-4">
      <FormField id="pw" label="Master password" required>
        <PasswordInput
          id="pw"
          bind:value={password}
          autocomplete="current-password"
          autofocus
          disabled={submitting}
        />
      </FormField>

      {#if error}
        <div
          class="rounded-md border border-rose-300 bg-rose-50 px-3 py-2 text-sm text-rose-900 dark:border-rose-900 dark:bg-rose-950 dark:text-rose-100"
        >
          {error}
        </div>
      {/if}

      <div class="flex justify-end pt-2">
        <Button type="submit" loading={submitting} disabled={submitting}>Unlock</Button>
      </div>
    </form>
  </Card>
</div>
