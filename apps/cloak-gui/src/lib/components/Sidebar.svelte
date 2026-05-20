<script lang="ts">
  import { router, navigate, type RoutePath } from '$lib/router.svelte';
  import { vaultStore } from '$lib/stores/vault.svelte';
  import { palette } from '$lib/stores/palette.svelte';
  import VaultStateChip from './VaultStateChip.svelte';

  const modKey =
    typeof navigator !== 'undefined' && /Mac|iPhone|iPad/.test(navigator.platform) ? '⌘' : 'Ctrl';

  interface Item {
    path: RoutePath;
    label: string;
    icon: string;
  }

  const items: Item[] = [
    { path: 'dashboard', label: 'Dashboard', icon: 'home' },
    { path: 'secrets', label: 'Secrets', icon: 'key' },
    { path: 'endpoints', label: 'Endpoints', icon: 'plug' },
    { path: 'tokens', label: 'Tokens', icon: 'tag' },
    { path: 'audit', label: 'Audit log', icon: 'list' },
  ];

  const icons: Record<string, string> = {
    home: 'M3 12L12 3l9 9M5 10v10h14V10',
    key: 'M21 2l-9.6 9.6M15.5 7.5l3 3M11.4 11.6a5 5 0 1 1-7 7 5 5 0 0 1 7-7z',
    plug: 'M9 7V2M15 7V2M5 11h14v3a7 7 0 0 1-7 7 7 7 0 0 1-7-7v-3zM12 21v3',
    tag: 'M20.6 13.4L13 21l-9-9 7.6-7.6h9z M7 7h.01',
    list: 'M8 6h13M8 12h13M8 18h13M3 6h.01M3 12h.01M3 18h.01',
  };
</script>

<aside
  class="flex h-full w-56 shrink-0 flex-col border-r border-zinc-200 bg-zinc-50 dark:border-zinc-800 dark:bg-zinc-950"
>
  <div class="px-5 py-4">
    <h1 class="font-mono text-base font-semibold tracking-tight text-zinc-900 dark:text-zinc-100">
      cloak
    </h1>
    <p class="text-xs text-zinc-500 dark:text-zinc-400">Local secret broker</p>
  </div>

  <nav class="flex-1 px-3" aria-label="Main">
    <ul class="flex flex-col gap-0.5">
      {#each items as item (item.path)}
        {@const active = router.isPrefix(item.path)}
        <li>
          <button
            type="button"
            onclick={() => navigate(item.path)}
            aria-current={active ? 'page' : undefined}
            class="
              flex w-full items-center gap-2.5 rounded-md px-3 py-2 text-sm transition
              {active
                ? 'bg-zinc-200 font-medium text-zinc-900 dark:bg-zinc-800 dark:text-zinc-100'
                : 'text-zinc-600 hover:bg-zinc-100 hover:text-zinc-900 dark:text-zinc-400 dark:hover:bg-zinc-800 dark:hover:text-zinc-200'}
            "
          >
            <svg
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="1.75"
              stroke-linecap="round"
              stroke-linejoin="round"
              class="size-4"
              aria-hidden="true"
            >
              <path d={icons[item.icon]} />
            </svg>
            {item.label}
          </button>
        </li>
      {/each}
    </ul>
  </nav>

  <div class="flex flex-col gap-2 border-t border-zinc-200 px-3 py-3 dark:border-zinc-800">
    <button
      type="button"
      onclick={() => palette.show()}
      class="
        flex items-center justify-between gap-2 rounded-md px-2 py-1.5 text-xs
        text-zinc-500 transition hover:bg-zinc-100 hover:text-zinc-900
        dark:text-zinc-400 dark:hover:bg-zinc-800 dark:hover:text-zinc-200
      "
    >
      <span class="flex items-center gap-1.5">
        <svg
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="1.75"
          class="size-3.5"
        >
          <circle cx="11" cy="11" r="7" />
          <path d="M21 21l-4.3-4.3" />
        </svg>
        Search…
      </span>
      <kbd
        class="rounded border border-zinc-300 bg-white px-1.5 text-[10px] font-medium text-zinc-600 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-300"
      >
        {modKey}K
      </kbd>
    </button>
    {#if vaultStore.phase.kind === 'ok'}
      <div class="px-2">
        <VaultStateChip state={vaultStore.phase.status.state} />
      </div>
    {/if}
  </div>
</aside>
