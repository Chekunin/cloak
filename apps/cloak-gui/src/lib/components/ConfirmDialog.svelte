<script lang="ts">
  /**
   * Accessible confirmation dialog.
   *
   * Why a custom modal instead of `window.confirm`?
   *
   * - `window.confirm` is non-functional in some Tauri webview/CSP combos.
   * - Even when it works, it's an OS-styled blocking dialog with no theming
   *   or destructive-action emphasis.
   * - We need to require typing the resource name for the most dangerous
   *   operations — `window.confirm` can't do that.
   *
   * Usage:
   *   let dialog = $state<ConfirmConfig | null>(null);
   *   <ConfirmDialog config={dialog} onClose={(ok) => { dialog = null; if (ok) doIt(); }} />
   */

  import Button from './Button.svelte';
  import Input from './Input.svelte';

  export interface ConfirmConfig {
    title: string;
    message: string;
    /** Optional second line in smaller text. */
    hint?: string;
    /** Visual emphasis on the confirm button. */
    variant?: 'primary' | 'danger';
    /** Custom button labels. */
    confirmLabel?: string;
    cancelLabel?: string;
    /**
     * When set, the user must type this string into a confirmation field
     * before the confirm button is enabled. Use for the most dangerous
     * destructive actions.
     */
    requireTyping?: string;
  }

  interface Props {
    config: ConfirmConfig | null;
    onClose: (confirmed: boolean) => void;
  }

  const { config, onClose }: Props = $props();

  let typedConfirmation = $state('');
  let dialogEl: HTMLDivElement | undefined = $state();
  let prevFocus: Element | null = null;

  const confirmEnabled = $derived(
    !config?.requireTyping || typedConfirmation === config.requireTyping,
  );

  // Track open/close transitions to manage focus + reset typed input.
  $effect(() => {
    if (config) {
      typedConfirmation = '';
      prevFocus = document.activeElement;
      queueMicrotask(() => {
        const el = dialogEl;
        if (!el) return;
        const target =
          el.querySelector<HTMLElement>('input, button[type="submit"]') ??
          el.querySelector<HTMLButtonElement>('button');
        target?.focus();
      });
    } else if (prevFocus instanceof HTMLElement) {
      prevFocus.focus();
      prevFocus = null;
    }
  });

  // Esc cancels, Enter confirms (when allowed).
  function onKeyDown(e: KeyboardEvent) {
    if (!config) return;
    if (e.key === 'Escape') {
      e.preventDefault();
      onClose(false);
    } else if (e.key === 'Enter' && confirmEnabled && !(e.target instanceof HTMLTextAreaElement)) {
      e.preventDefault();
      onClose(true);
    }
  }
</script>

<svelte:window onkeydown={onKeyDown} />

{#if config}
  <!-- Backdrop. Click-outside cancels. -->
  <div
    class="fixed inset-0 z-50 flex items-center justify-center bg-zinc-950/60 p-4 backdrop-blur-sm"
    role="presentation"
    onclick={(e) => {
      if (e.target === e.currentTarget) onClose(false);
    }}
    onkeydown={() => {}}
  >
    <div
      bind:this={dialogEl}
      role="dialog"
      aria-modal="true"
      aria-labelledby="confirm-title"
      aria-describedby="confirm-message"
      class="w-full max-w-md rounded-xl border border-zinc-200 bg-white shadow-xl dark:border-zinc-800 dark:bg-zinc-900"
    >
      <div class="px-6 pb-2 pt-6">
        <h2 id="confirm-title" class="text-base font-semibold text-zinc-900 dark:text-zinc-100">
          {config.title}
        </h2>
        <p id="confirm-message" class="mt-1 text-sm text-zinc-600 dark:text-zinc-400">
          {config.message}
        </p>
        {#if config.hint}
          <p class="mt-2 text-xs text-zinc-500 dark:text-zinc-400">{config.hint}</p>
        {/if}
      </div>

      {#if config.requireTyping}
        <div class="px-6 pb-4">
          <label
            for="confirm-typed"
            class="text-xs font-medium text-zinc-700 dark:text-zinc-300"
          >
            Type <code class="select-text font-mono">{config.requireTyping}</code> to confirm.
          </label>
          <div class="mt-1.5">
            <Input id="confirm-typed" bind:value={typedConfirmation} autofocus />
          </div>
        </div>
      {/if}

      <div
        class="flex items-center justify-end gap-2 rounded-b-xl border-t border-zinc-200 bg-zinc-50/60 px-6 py-3 dark:border-zinc-800 dark:bg-zinc-900/40"
      >
        <Button variant="ghost" onclick={() => onClose(false)}>
          {config.cancelLabel ?? 'Cancel'}
        </Button>
        <Button
          variant={config.variant ?? 'primary'}
          onclick={() => onClose(true)}
          disabled={!confirmEnabled}
        >
          {config.confirmLabel ?? 'Confirm'}
        </Button>
      </div>
    </div>
  </div>
{/if}
