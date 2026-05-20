<script lang="ts">
  import type { HTMLInputAttributes } from 'svelte/elements';

  interface Props {
    id?: string;
    value: string;
    placeholder?: string;
    disabled?: boolean;
    autocomplete?: HTMLInputAttributes['autocomplete'];
    autofocus?: boolean;
    invalid?: boolean;
    onkeydown?: (e: KeyboardEvent) => void;
  }

  let {
    id,
    value = $bindable(),
    placeholder,
    disabled = false,
    autocomplete = 'current-password',
    autofocus = false,
    invalid = false,
    onkeydown,
  }: Props = $props();

  let revealed = $state(false);
</script>

<div class="relative">
  <input
    {id}
    type={revealed ? 'text' : 'password'}
    {placeholder}
    {disabled}
    {autocomplete}
    bind:value
    onkeydown={(e) => onkeydown?.(e)}
    aria-invalid={invalid}
    {@attach (el) => {
      if (autofocus && el instanceof HTMLInputElement) {
        el.focus();
      }
    }}
    class="
      w-full select-text rounded-md border bg-white px-3 py-2 pr-10 text-sm
      text-zinc-900 placeholder:text-zinc-400 transition
      focus:outline-none focus:ring-2 focus:ring-offset-1
      disabled:cursor-not-allowed disabled:opacity-50
      dark:bg-zinc-900 dark:text-zinc-100 dark:placeholder:text-zinc-500
      {invalid
      ? 'border-rose-400 focus:ring-rose-400 dark:border-rose-700'
      : 'border-zinc-300 focus:border-zinc-500 focus:ring-zinc-400 dark:border-zinc-700 dark:focus:border-zinc-500 dark:focus:ring-zinc-500'}
    "
  />
  <button
    type="button"
    onclick={() => (revealed = !revealed)}
    aria-label={revealed ? 'Hide password' : 'Show password'}
    class="
      absolute inset-y-0 right-0 flex items-center px-3 text-zinc-400
      transition hover:text-zinc-700 dark:hover:text-zinc-200
    "
  >
    {#if revealed}
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="size-4">
        <path d="M3 3l18 18M10.6 10.6A2 2 0 0 0 12 14a2 2 0 0 0 1.4-.6M9.9 5.1A10 10 0 0 1 12 5c6 0 10 7 10 7a17.6 17.6 0 0 1-3.4 4.3M6.6 6.6A17.6 17.6 0 0 0 2 12s4 7 10 7c1.2 0 2.3-.3 3.4-.7"/>
      </svg>
    {:else}
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="size-4">
        <path d="M2 12s4-7 10-7 10 7 10 7-4 7-10 7S2 12 2 12z"/>
        <circle cx="12" cy="12" r="3"/>
      </svg>
    {/if}
  </button>
</div>
