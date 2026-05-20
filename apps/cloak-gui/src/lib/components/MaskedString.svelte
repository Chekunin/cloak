<script lang="ts">
  import CopyButton from './CopyButton.svelte';

  interface Props {
    value: string;
    /** When true (default), starts masked and offers a Reveal toggle. */
    sensitive?: boolean;
    /** Toast label for the copy action. */
    label?: string;
    /**
     * When true, the masked state is a fixed dot string that reveals nothing
     * — not even the value's length, prefix or suffix. Use for credentials
     * (passwords, keys); the default prefix/suffix preview suits connection
     * strings, where a glimpse aids recognition.
     */
    fullMask?: boolean;
  }

  const { value, sensitive = true, label = 'Copied', fullMask = false }: Props = $props();

  // Read `sensitive` once via a plain function call so svelte-check doesn't
  // flag "reference only captures initial value" — the only behaviour we
  // want is exactly that initial capture.
  function initialReveal(): boolean {
    return !sensitive;
  }
  let revealed = $state<boolean>(initialReveal());

  // Fixed-length mask: a compact, constant-width preview. Crucially it does
  // NOT scale with the secret's length — a long connection string masked
  // with one bullet per character produced a giant multi-line blob that
  // broke the row layout. A fixed mask also avoids leaking the length.
  //
  // With `fullMask`, even the prefix/suffix preview is dropped — the masked
  // form is purely dots, so a credential leaks nothing while hidden.
  function masked(s: string): string {
    if (fullMask || s.length <= 12) return '••••••••';
    return `${s.slice(0, 6)}••••••••${s.slice(-4)}`;
  }
</script>

<!-- items-start: keep the controls pinned to the top, never floating into
     the middle of a wrapped revealed value. -->
<div
  class="flex items-start gap-2 rounded-md border border-zinc-200 bg-zinc-50 px-3 py-2 dark:border-zinc-800 dark:bg-zinc-900"
>
  <!-- min-w-0 is the flexbox fix: without it a flex child keeps its
       content's min-content width and overflows under the sibling controls.
       With it, break-all content wraps inside the available width. -->
  <code
    class="min-w-0 flex-1 select-text break-all py-0.5 font-mono text-xs leading-relaxed text-zinc-700 dark:text-zinc-300"
  >
    {revealed ? value : masked(value)}
  </code>
  <div class="flex shrink-0 items-center gap-1">
    {#if sensitive}
      <button
        type="button"
        onclick={() => (revealed = !revealed)}
        aria-label={revealed ? 'Hide' : 'Reveal'}
        class="rounded p-1 text-zinc-400 transition hover:bg-zinc-100 hover:text-zinc-700 dark:hover:bg-zinc-800 dark:hover:text-zinc-200"
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
    {/if}
    <CopyButton {value} {label} sensitive={sensitive} iconOnly />
  </div>
</div>
