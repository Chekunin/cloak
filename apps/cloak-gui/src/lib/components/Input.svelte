<script lang="ts">
  import type { HTMLInputAttributes } from 'svelte/elements';

  interface Props {
    id?: string;
    type?: 'text' | 'password' | 'number' | 'email';
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
    type = 'text',
    value = $bindable(),
    placeholder,
    disabled = false,
    autocomplete,
    autofocus = false,
    invalid = false,
    onkeydown,
  }: Props = $props();
</script>

<input
  {id}
  {type}
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
    w-full select-text rounded-md border bg-white px-3 py-2 text-sm
    text-zinc-900 placeholder:text-zinc-400 transition
    focus:outline-none focus:ring-2 focus:ring-offset-1
    disabled:cursor-not-allowed disabled:opacity-50
    dark:bg-zinc-900 dark:text-zinc-100 dark:placeholder:text-zinc-500
    {invalid
    ? 'border-rose-400 focus:ring-rose-400 dark:border-rose-700'
    : 'border-zinc-300 focus:border-zinc-500 focus:ring-zinc-400 dark:border-zinc-700 dark:focus:border-zinc-500 dark:focus:ring-zinc-500'}
  "
/>
