<script lang="ts">
  /**
   * Editable list of key/value pairs.
   *
   * Used for HTTP header injection rules, header injection values, and
   * `strip_request_headers` (where values are unused — see `valueless`).
   */

  import Input from './Input.svelte';
  import PasswordInput from './PasswordInput.svelte';

  interface Pair {
    key: string;
    value: string;
  }

  interface Props {
    pairs: Pair[];
    keyPlaceholder?: string;
    valuePlaceholder?: string;
    /** Treat values as sensitive — render via <PasswordInput> with reveal toggle. */
    sensitiveValues?: boolean;
    /** When true, render only the key column (for header strip lists). */
    valueless?: boolean;
    addLabel?: string;
  }

  let {
    pairs = $bindable(),
    keyPlaceholder = 'Key',
    valuePlaceholder = 'Value',
    sensitiveValues = false,
    valueless = false,
    addLabel = 'Add row',
  }: Props = $props();

  function addRow() {
    pairs = [...pairs, { key: '', value: '' }];
  }

  function removeRow(idx: number) {
    pairs = pairs.filter((_, i) => i !== idx);
  }
</script>

<div class="flex flex-col gap-2">
  {#each pairs as pair, idx (idx)}
    <div class="flex items-start gap-2">
      <div class="flex-1">
        <Input bind:value={pair.key} placeholder={keyPlaceholder} />
      </div>
      {#if !valueless}
        <div class="flex-1">
          {#if sensitiveValues}
            <PasswordInput bind:value={pair.value} placeholder={valuePlaceholder} autocomplete="off" />
          {:else}
            <Input bind:value={pair.value} placeholder={valuePlaceholder} />
          {/if}
        </div>
      {/if}
      <button
        type="button"
        onclick={() => removeRow(idx)}
        class="mt-1 rounded p-1.5 text-zinc-400 transition hover:bg-rose-50 hover:text-rose-600 dark:hover:bg-rose-950 dark:hover:text-rose-400"
        aria-label="Remove row"
      >
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="size-4">
          <path d="M6 6l12 12M6 18L18 6" />
        </svg>
      </button>
    </div>
  {/each}
  <button
    type="button"
    onclick={addRow}
    class="self-start rounded-md border border-dashed border-zinc-300 px-3 py-1.5 text-xs text-zinc-600 transition hover:border-zinc-400 hover:text-zinc-900 dark:border-zinc-700 dark:text-zinc-400 dark:hover:border-zinc-600 dark:hover:text-zinc-100"
  >
    + {addLabel}
  </button>
</div>
