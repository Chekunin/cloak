<script lang="ts">
  /**
   * "Set up the GUI's bearer token" panel.
   *
   * Shown when the daemon is reachable + vault is unlocked + the GUI has no
   * authenticated session yet. Two import paths:
   *
   *   1. Re-read ~/.cloak/cli_token — for the user who just ran
   *      `cloak token create --save`.
   *   2. Paste an existing plaintext token — for the user who has one but no
   *      saved file.
   *
   * On success, refreshes the connection store so every other view immediately
   * sees `hasToken === true`.
   */

  import { daemon, isCommandError } from '$lib/api';
  import { connection } from '$lib/stores/connection.svelte';
  import { toasts } from '$lib/stores/toasts.svelte';
  import Button from './Button.svelte';
  import Card from './Card.svelte';
  import FormField from './FormField.svelte';
  import PasswordInput from './PasswordInput.svelte';

  let working = $state(false);
  let pasted = $state('');
  let pasteError = $state<string | null>(null);

  function explain(err: unknown): string {
    if (isCommandError(err)) {
      return err.hint ? `${err.message} — ${err.hint}` : err.message;
    }
    return err instanceof Error ? err.message : String(err);
  }

  async function reloadCli() {
    working = true;
    try {
      const found = await daemon.reloadCliToken();
      if (found) {
        await connection.refresh();
        toasts.success('GUI authorised', 'Loaded the token from ~/.cloak/cli_token.');
      } else {
        toasts.error(
          'No CLI token on disk',
          "Run `cloak token create --name gui --save` in a terminal, then click again.",
        );
      }
    } catch (err) {
      toasts.error('Could not load token', explain(err));
    } finally {
      working = false;
    }
  }

  async function submitPasted(e: SubmitEvent) {
    e.preventDefault();
    pasteError = null;
    if (!pasted.trim()) {
      pasteError = 'Paste a token first.';
      return;
    }
    working = true;
    try {
      await daemon.setToken(pasted);
      await connection.refresh();
      pasted = '';
      toasts.success('GUI authorised');
    } catch (err) {
      pasteError = explain(err);
    } finally {
      working = false;
    }
  }
</script>

<Card
  title="Set up GUI authentication"
  description="This GUI doesn't have a token yet. Pick one of the options below."
>
  <div class="flex flex-col gap-6">
    <section>
      <h3 class="text-sm font-medium text-zinc-700 dark:text-zinc-300">
        1. Use a token saved by the CLI
      </h3>
      <p class="mt-1 text-xs text-zinc-500 dark:text-zinc-400">
        If you've run <code class="select-text">cloak token create --name gui --save</code>, the
        plaintext is at <code class="select-text">~/.cloak/cli_token</code>. Click to load it.
      </p>
      <div class="mt-2">
        <Button variant="secondary" onclick={reloadCli} disabled={working}>
          Re-read ~/.cloak/cli_token
        </Button>
      </div>
    </section>

    <hr class="border-zinc-200 dark:border-zinc-800" />

    <section>
      <h3 class="text-sm font-medium text-zinc-700 dark:text-zinc-300">
        2. Paste an existing token
      </h3>
      <p class="mt-1 text-xs text-zinc-500 dark:text-zinc-400">
        The full <code class="select-text">&lt;id&gt;.&lt;base64&gt;</code> string Cloak issued.
        Stored in this process's memory only — not written to disk.
      </p>
      <form onsubmit={submitPasted} class="mt-2 flex flex-col gap-3">
        <FormField id="gui-token" label="Token" error={pasteError ?? undefined}>
          <PasswordInput
            id="gui-token"
            bind:value={pasted}
            autocomplete="off"
            placeholder="01ABC...XYZ.somerandombase64string"
          />
        </FormField>
        <div class="flex justify-end">
          <Button type="submit" loading={working} disabled={working}>Authorise GUI</Button>
        </div>
      </form>
    </section>
  </div>
</Card>
