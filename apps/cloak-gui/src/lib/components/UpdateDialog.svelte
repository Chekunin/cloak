<script lang="ts">
  /**
   * Modal for the "Check for Updates" flow. Always mounted (in App.svelte);
   * visible only while `update.phase` is non-null.
   */
  import { update } from '$lib/stores/update.svelte';
  import Button from './Button.svelte';

  const phase = $derived(update.phase);

  /** The install step can't be interrupted — the app is about to relaunch. */
  const dismissable = $derived(phase !== null && phase.kind !== 'installing');

  /** Dismiss only on a click of the backdrop itself, not a bubbled child click. */
  function onBackdrop(ev: MouseEvent) {
    if (dismissable && ev.target === ev.currentTarget) update.dismiss();
  }

  function onKeydown(ev: KeyboardEvent) {
    if (ev.key === 'Escape' && dismissable) update.dismiss();
  }
</script>

<svelte:window onkeydown={onKeydown} />

{#if phase}
  <div
    class="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4"
    role="presentation"
    onclick={onBackdrop}
  >
    <div
      class="w-full max-w-md rounded-xl border border-zinc-200 bg-white p-6 shadow-xl dark:border-zinc-800 dark:bg-zinc-900"
      role="dialog"
      aria-modal="true"
      tabindex="-1"
    >
      {#if phase.kind === 'checking'}
        <div class="flex items-center gap-3">
          <div
            class="size-5 animate-spin rounded-full border-2 border-zinc-300 border-t-zinc-700 dark:border-zinc-700 dark:border-t-zinc-300"
          ></div>
          <p class="text-sm text-zinc-700 dark:text-zinc-300">Checking for updates…</p>
        </div>
      {:else if phase.kind === 'uptodate'}
        <h2 class="text-lg font-semibold text-zinc-900 dark:text-zinc-100">You're up to date</h2>
        <p class="mt-1 text-sm text-zinc-500 dark:text-zinc-400">
          Cloak is running the latest version.
        </p>
        <div class="mt-5 flex justify-end">
          <Button variant="secondary" onclick={() => update.dismiss()}>Close</Button>
        </div>
      {:else if phase.kind === 'available'}
        <h2 class="text-lg font-semibold text-zinc-900 dark:text-zinc-100">Update available</h2>
        <p class="mt-1 text-sm text-zinc-500 dark:text-zinc-400">
          Version {phase.info.version} is available — you have {phase.info.current_version}.
        </p>
        {#if phase.info.notes}
          <pre
            class="mt-3 max-h-48 overflow-y-auto whitespace-pre-wrap rounded-md border border-zinc-200 bg-zinc-50 p-3 text-xs text-zinc-700 dark:border-zinc-800 dark:bg-zinc-950 dark:text-zinc-300">{phase
              .info.notes}</pre>
        {/if}
        <div class="mt-5 flex justify-end gap-2">
          <Button variant="ghost" onclick={() => update.dismiss()}>Later</Button>
          <Button onclick={() => update.install()}>Install &amp; Restart</Button>
        </div>
      {:else if phase.kind === 'installing'}
        <div class="flex items-center gap-3">
          <div
            class="size-5 animate-spin rounded-full border-2 border-zinc-300 border-t-zinc-700 dark:border-zinc-700 dark:border-t-zinc-300"
          ></div>
          <div>
            <p class="text-sm font-medium text-zinc-900 dark:text-zinc-100">
              Downloading and installing…
            </p>
            <p class="text-xs text-zinc-500 dark:text-zinc-400">Cloak will restart automatically.</p>
          </div>
        </div>
      {:else if phase.kind === 'error'}
        <h2 class="text-lg font-semibold text-zinc-900 dark:text-zinc-100">Update failed</h2>
        <p
          class="mt-2 rounded-md border border-rose-300 bg-rose-50 px-3 py-2 text-sm text-rose-900 dark:border-rose-900 dark:bg-rose-950 dark:text-rose-100"
        >
          {phase.message}
        </p>
        <div class="mt-5 flex justify-end">
          <Button variant="secondary" onclick={() => update.dismiss()}>Close</Button>
        </div>
      {/if}
    </div>
  </div>
{/if}
