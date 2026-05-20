<script lang="ts">
  import type { Snippet } from 'svelte';

  interface Props {
    title?: string;
    description?: string;
    children: Snippet;
    footer?: Snippet;
  }

  const { title, description, children, footer }: Props = $props();
</script>

<!-- overflow-hidden clips the header/footer fills to the rounded corners. -->
<section
  class="overflow-hidden rounded-xl border border-zinc-200 bg-white shadow-sm dark:border-zinc-800 dark:bg-zinc-900"
>
  {#if title || description}
    <header class="border-b border-zinc-200 px-6 py-4 dark:border-zinc-800">
      {#if title}
        <h2 class="text-base font-semibold text-zinc-900 dark:text-zinc-100">
          {title}
        </h2>
      {/if}
      {#if description}
        <p class="mt-1 text-sm text-zinc-500 dark:text-zinc-400">{description}</p>
      {/if}
    </header>
  {/if}
  <div class="px-6 py-5">{@render children()}</div>
  {#if footer}
    <footer class="border-t border-zinc-200 bg-zinc-50/60 px-6 py-4 dark:border-zinc-800 dark:bg-zinc-900/30">
      {@render footer()}
    </footer>
  {/if}
</section>
