<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { router } from '$lib/router.svelte';
  import { exec, isCommandError } from '$lib/api';
  import type { ExecResult } from '$lib/api';
  import { secretsStore } from '$lib/stores/secrets.svelte';
  import { vaultStore } from '$lib/stores/vault.svelte';
  import Button from '$lib/components/Button.svelte';
  import Card from '$lib/components/Card.svelte';
  import EmptyState from '$lib/components/EmptyState.svelte';

  const vaultUnlocked = $derived(
    vaultStore.phase.kind === 'ok' && vaultStore.phase.status.state === 'unlocked',
  );

  /** All secrets, env types first — exec is most useful for those. */
  const secrets = $derived.by(() => {
    if (secretsStore.phase.kind !== 'ok') return [];
    return [...secretsStore.phase.items].sort((a, b) => {
      if (a.type === 'env' && b.type !== 'env') return -1;
      if (b.type === 'env' && a.type !== 'env') return 1;
      return a.name.localeCompare(b.name);
    });
  });

  let selected = $state('');
  let command = $state('');
  let running = $state(false);
  let result = $state<ExecResult | null>(null);
  let runError = $state<string | null>(null);

  onMount(() => {
    if (vaultUnlocked) secretsStore.start();
  });
  onDestroy(() => secretsStore.stop());

  $effect(() => {
    if (vaultUnlocked) secretsStore.start();
    else secretsStore.stop();
  });

  // Pick an initial secret: the one named in the route (#run:<name>), else the
  // first env secret, else the first secret of any type.
  $effect(() => {
    if (selected || secrets.length === 0) return;
    const fromRoute = router.route.params[0];
    selected =
      (fromRoute && secrets.find((s) => s.name === fromRoute)?.name) ??
      secrets.find((s) => s.type === 'env')?.name ??
      secrets[0].name;
  });

  async function run() {
    if (!selected || !command.trim() || running) return;
    running = true;
    runError = null;
    result = null;
    try {
      result = await exec.run(selected, command);
    } catch (err) {
      runError = isCommandError(err)
        ? err.hint
          ? `${err.message} — ${err.hint}`
          : err.message
        : err instanceof Error
          ? err.message
          : String(err);
    } finally {
      running = false;
    }
  }

  function onKeydown(ev: KeyboardEvent) {
    if (ev.key === 'Enter' && (ev.metaKey || ev.ctrlKey)) {
      ev.preventDefault();
      void run();
    }
  }
</script>

<div class="flex flex-col gap-6 p-8">
  <header>
    <h1 class="text-2xl font-semibold tracking-tight text-zinc-900 dark:text-zinc-100">Run</h1>
    <p class="text-sm text-zinc-500 dark:text-zinc-400">
      Run a command with a secret's credentials injected — the GUI equivalent of
      <code class="font-mono text-xs">cloak exec</code>. Cloak opens an endpoint, runs the command
      with the variables layered into its environment, then closes the endpoint.
    </p>
  </header>

  {#if !vaultUnlocked}
    <Card>
      <div class="text-center text-sm text-zinc-500 dark:text-zinc-400">
        Unlock the vault to run commands.
      </div>
    </Card>
  {:else if secretsStore.phase.kind === 'loading'}
    <Card><p class="text-sm text-zinc-500 dark:text-zinc-400">Loading…</p></Card>
  {:else if secrets.length === 0}
    <EmptyState
      title="No secrets yet"
      description="Add a secret first — an Env secret is the natural fit for CLI tools."
    />
  {:else}
    <Card>
      <div class="flex flex-col gap-4">
        <div class="grid grid-cols-1 gap-4 sm:grid-cols-[14rem_1fr]">
          <label class="flex flex-col gap-1.5">
            <span class="text-sm font-medium text-zinc-700 dark:text-zinc-300">Secret</span>
            <select
              bind:value={selected}
              class="w-full rounded-md border border-zinc-300 bg-white px-3 py-2 text-sm dark:border-zinc-700 dark:bg-zinc-900"
            >
              {#each secrets as s (s.id)}
                <option value={s.name}>{s.name} ({s.type})</option>
              {/each}
            </select>
          </label>

          <label class="flex flex-col gap-1.5">
            <span class="text-sm font-medium text-zinc-700 dark:text-zinc-300">Command</span>
            <input
              bind:value={command}
              onkeydown={onKeydown}
              placeholder="aws s3 ls"
              spellcheck="false"
              autocapitalize="off"
              autocomplete="off"
              class="w-full rounded-md border border-zinc-300 bg-white px-3 py-2 font-mono text-sm dark:border-zinc-700 dark:bg-zinc-900"
            />
          </label>
        </div>

        <div class="flex items-center justify-between gap-3">
          <p class="text-xs text-zinc-500 dark:text-zinc-400">
            Runs through your shell. ⌘/Ctrl+Enter to run.
          </p>
          <Button onclick={run} loading={running} disabled={running || !command.trim()}>
            Run
          </Button>
        </div>
      </div>
    </Card>

    {#if runError}
      <Card>
        <div
          class="rounded-md border border-rose-300 bg-rose-50 p-4 text-sm text-rose-900 dark:border-rose-900 dark:bg-rose-950 dark:text-rose-100"
        >
          {runError}
        </div>
      </Card>
    {/if}

    {#if result}
      <Card>
        <div class="flex flex-col gap-3">
          <div class="flex flex-wrap items-center gap-2">
            <span
              class="rounded-full px-2 py-0.5 text-xs font-medium {result.exit_code === 0
                ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-950 dark:text-emerald-300'
                : 'bg-rose-100 text-rose-700 dark:bg-rose-950 dark:text-rose-300'}"
            >
              exit {result.exit_code}
            </span>
            {#each result.env_var_names as name (name)}
              <span
                class="rounded-full bg-zinc-100 px-2 py-0.5 font-mono text-xs text-zinc-600 dark:bg-zinc-800 dark:text-zinc-400"
              >
                {name}
              </span>
            {/each}
          </div>

          {#if result.stdout}
            <div>
              <div class="mb-1 text-xs font-medium uppercase tracking-wider text-zinc-500 dark:text-zinc-400">
                stdout
              </div>
              <pre
                class="overflow-x-auto rounded-md bg-zinc-950 p-3 font-mono text-xs leading-relaxed text-zinc-100">{result.stdout}</pre>
            </div>
          {/if}
          {#if result.stderr}
            <div>
              <div class="mb-1 text-xs font-medium uppercase tracking-wider text-zinc-500 dark:text-zinc-400">
                stderr
              </div>
              <pre
                class="overflow-x-auto rounded-md bg-zinc-950 p-3 font-mono text-xs leading-relaxed text-rose-300">{result.stderr}</pre>
            </div>
          {/if}
          {#if !result.stdout && !result.stderr}
            <p class="text-sm text-zinc-500 dark:text-zinc-400">(no output)</p>
          {/if}
          {#if result.truncated}
            <p class="text-xs text-amber-600 dark:text-amber-400">Output was truncated.</p>
          {/if}
        </div>
      </Card>
    {/if}
  {/if}
</div>
