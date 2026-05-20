<script lang="ts">
  import type { Snippet } from 'svelte';

  type Variant = 'primary' | 'secondary' | 'ghost' | 'danger';

  interface Props {
    variant?: Variant;
    type?: 'button' | 'submit';
    disabled?: boolean;
    loading?: boolean;
    fullWidth?: boolean;
    onclick?: (e: MouseEvent) => void;
    children: Snippet;
  }

  const {
    variant = 'primary',
    type = 'button',
    disabled = false,
    loading = false,
    fullWidth = false,
    onclick,
    children,
  }: Props = $props();

  const palette: Record<Variant, string> = {
    primary:
      'bg-zinc-900 text-white hover:bg-zinc-800 focus-visible:outline-zinc-900 dark:bg-zinc-50 dark:text-zinc-900 dark:hover:bg-zinc-200 dark:focus-visible:outline-zinc-50',
    secondary:
      'bg-zinc-100 text-zinc-900 hover:bg-zinc-200 focus-visible:outline-zinc-400 dark:bg-zinc-800 dark:text-zinc-100 dark:hover:bg-zinc-700 dark:focus-visible:outline-zinc-500',
    ghost:
      'bg-transparent text-zinc-700 hover:bg-zinc-100 focus-visible:outline-zinc-400 dark:text-zinc-300 dark:hover:bg-zinc-800 dark:focus-visible:outline-zinc-500',
    danger:
      'bg-rose-600 text-white hover:bg-rose-700 focus-visible:outline-rose-600',
  };
</script>

<button
  {type}
  disabled={disabled || loading}
  class="
    inline-flex items-center justify-center gap-2 rounded-md px-3 py-2 text-sm font-medium
    transition disabled:cursor-not-allowed disabled:opacity-50
    focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2
    {palette[variant]}
    {fullWidth ? 'w-full' : ''}
  "
  onclick={(e) => onclick?.(e)}
>
  {#if loading}
    <svg
      class="size-4 animate-spin"
      xmlns="http://www.w3.org/2000/svg"
      fill="none"
      viewBox="0 0 24 24"
      aria-hidden="true"
    >
      <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"
      ></circle>
      <path
        class="opacity-75"
        fill="currentColor"
        d="M4 12a8 8 0 0 1 8-8V0C5.4 0 0 5.4 0 12h4zm2 5.3A8 8 0 0 1 4 12H0c0 3 1.1 5.8 3 7.9l3-2.6z"
      ></path>
    </svg>
  {/if}
  {@render children()}
</button>
