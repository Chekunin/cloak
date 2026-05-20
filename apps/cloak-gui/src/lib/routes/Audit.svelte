<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { audit as auditApi, isCommandError } from '$lib/api';
  import type { AuditEntry } from '$lib/api';
  import { formatDate, timeAgo } from '$lib/format';
  import Card from '$lib/components/Card.svelte';
  import Input from '$lib/components/Input.svelte';
  import FormField from '$lib/components/FormField.svelte';

  let entries = $state<AuditEntry[]>([]);
  let errorMessage = $state<string | null>(null);
  let loading = $state(true);

  // Filters.
  let typePrefix = $state('');
  let secretFilter = $state('');
  let sinceMinutes = $state(0); // 0 = no time filter

  // Limit fetched from the daemon. Higher = more memory; the client filters
  // further before rendering.
  const limit = 500;

  let timer: number | null = null;

  onMount(() => {
    void refresh();
    timer = window.setInterval(() => void refresh(), 2000);
  });
  onDestroy(() => {
    if (timer !== null) window.clearInterval(timer);
  });

  async function refresh() {
    try {
      entries = await auditApi.tail(limit);
      errorMessage = null;
    } catch (err) {
      errorMessage = isCommandError(err)
        ? err.message
        : err instanceof Error
          ? err.message
          : String(err);
    } finally {
      loading = false;
    }
  }

  const filtered = $derived.by(() => {
    const sinceTime = sinceMinutes > 0 ? Date.now() - sinceMinutes * 60 * 1000 : 0;
    const tp = typePrefix.trim();
    const sf = secretFilter.trim().toLowerCase();
    return [...entries].reverse().filter((e) => {
      const ev = String(e.event ?? '');
      if (tp && !ev.startsWith(tp)) return false;
      if (sf) {
        const name = String(e.secret_name ?? '').toLowerCase();
        if (!name.includes(sf)) return false;
      }
      if (sinceTime > 0) {
        const ts = String(e.ts ?? '');
        const t = Date.parse(ts);
        if (!Number.isNaN(t) && t < sinceTime) return false;
      }
      return true;
    });
  });

  // Track the selection by `seq` — a stable identity — not by object
  // reference. Each poll replaces every entry object, so an object-reference
  // selection would lose its list highlight (and freeze the detail pane on a
  // stale copy) after the first refresh.
  let selectedSeq = $state<number | null>(null);
  const selected = $derived.by(() => {
    if (selectedSeq === null) return null;
    return entries.find((e) => typeof e.seq === 'number' && e.seq === selectedSeq) ?? null;
  });

  /** Visual tone for each event type prefix. */
  function tone(ev: string): string {
    if (ev.startsWith('vault.'))
      return 'bg-violet-100 text-violet-800 dark:bg-violet-950 dark:text-violet-200';
    if (ev.includes('failed') || ev.includes('upstream_failed'))
      return 'bg-rose-100 text-rose-800 dark:bg-rose-950 dark:text-rose-200';
    if (ev.startsWith('endpoint.'))
      return 'bg-emerald-100 text-emerald-800 dark:bg-emerald-950 dark:text-emerald-200';
    if (ev.startsWith('secret.'))
      return 'bg-sky-100 text-sky-800 dark:bg-sky-950 dark:text-sky-200';
    if (ev.startsWith('token.'))
      return 'bg-amber-100 text-amber-800 dark:bg-amber-950 dark:text-amber-200';
    if (ev.startsWith('client.'))
      return 'bg-zinc-100 text-zinc-800 dark:bg-zinc-800 dark:text-zinc-200';
    return 'bg-zinc-100 text-zinc-800 dark:bg-zinc-800 dark:text-zinc-200';
  }

  const sinceOptions = [
    { value: 0, label: 'All time' },
    { value: 5, label: 'Last 5 min' },
    { value: 60, label: 'Last hour' },
    { value: 60 * 24, label: 'Last 24 hours' },
    { value: 60 * 24 * 7, label: 'Last 7 days' },
  ];
</script>

<div class="flex h-full flex-col gap-6 p-8">
  <header>
    <h1 class="text-2xl font-semibold tracking-tight text-zinc-900 dark:text-zinc-100">
      Audit log
    </h1>
    <p class="text-sm text-zinc-500 dark:text-zinc-400">
      Hash-chained record of every meaningful daemon event.
    </p>
  </header>

  <div class="grid grid-cols-1 gap-3 sm:grid-cols-3">
    <FormField id="typeFilter" label="Event type prefix" hint="e.g. endpoint.connection">
      <Input id="typeFilter" bind:value={typePrefix} placeholder="any" />
    </FormField>
    <FormField id="secretFilter" label="Secret name contains">
      <Input id="secretFilter" bind:value={secretFilter} placeholder="any" />
    </FormField>
    <FormField id="sinceFilter" label="Time window">
      <select
        id="sinceFilter"
        bind:value={sinceMinutes}
        class="w-full rounded-md border border-zinc-300 bg-white px-3 py-2 text-sm dark:border-zinc-700 dark:bg-zinc-900"
      >
        {#each sinceOptions as opt (opt.value)}
          <option value={opt.value}>{opt.label}</option>
        {/each}
      </select>
    </FormField>
  </div>

  <div class="grid min-h-0 flex-1 grid-cols-1 gap-4 lg:grid-cols-[1fr_22rem]">
    <Card>
      {#if loading && entries.length === 0}
        <p class="text-sm text-zinc-500 dark:text-zinc-400">Loading…</p>
      {:else if errorMessage}
        <div
          class="rounded-md border border-rose-300 bg-rose-50 p-4 text-sm text-rose-900 dark:border-rose-900 dark:bg-rose-950 dark:text-rose-100"
        >
          {errorMessage}
        </div>
      {:else if filtered.length === 0}
        <p class="text-sm text-zinc-500 dark:text-zinc-400">No events match the filters.</p>
      {:else}
        <!-- Internal scroll so the filter inputs above stay pinned. -->
        <ul class="max-h-[55vh] divide-y divide-zinc-200 overflow-y-auto dark:divide-zinc-800">
          {#each filtered as e (e.seq ?? e.ts)}
            {@const ev = String(e.event ?? '?')}
            {@const seq = typeof e.seq === 'number' ? e.seq : null}
            <li>
              <button
                type="button"
                onclick={() => (selectedSeq = seq)}
                class="
                  flex w-full items-center gap-3 px-2 py-2 text-left text-sm transition
                  hover:bg-zinc-50 dark:hover:bg-zinc-800
                  {seq !== null && seq === selectedSeq ? 'bg-zinc-100 dark:bg-zinc-800' : ''}
                "
              >
                <span class="shrink-0 rounded-full px-2 py-0.5 text-xs font-medium {tone(ev)}">
                  {ev}
                </span>
                {#if e.secret_name}
                  <span class="shrink-0 font-mono text-xs text-zinc-700 dark:text-zinc-300">
                    {e.secret_name}
                  </span>
                {/if}
                <span class="flex-1 truncate text-xs text-zinc-500 dark:text-zinc-400">
                  {#if e.remote_addr}
                    {e.remote_addr}
                  {:else if e.details}
                    {JSON.stringify(e.details)}
                  {/if}
                </span>
                <span class="shrink-0 text-xs text-zinc-400 dark:text-zinc-500">
                  {timeAgo(String(e.ts ?? ''))}
                </span>
              </button>
            </li>
          {/each}
        </ul>
      {/if}
    </Card>

    <Card title="Event detail">
      {#if !selected}
        <p class="text-sm text-zinc-500 dark:text-zinc-400">
          Click an event in the list to inspect.
        </p>
      {:else}
        <dl class="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-xs">
          {#if selected.event}
            <dt class="text-zinc-500 dark:text-zinc-400">Event</dt>
            <dd class="font-mono text-zinc-900 dark:text-zinc-100">{selected.event}</dd>
          {/if}
          {#if selected.ts}
            <dt class="text-zinc-500 dark:text-zinc-400">Time</dt>
            <dd class="text-zinc-700 dark:text-zinc-300">{formatDate(String(selected.ts))}</dd>
          {/if}
          {#if selected.seq !== undefined}
            <dt class="text-zinc-500 dark:text-zinc-400">Seq</dt>
            <dd class="font-mono text-zinc-700 dark:text-zinc-300">{selected.seq}</dd>
          {/if}
          {#if selected.secret_name}
            <dt class="text-zinc-500 dark:text-zinc-400">Secret</dt>
            <dd class="font-mono text-zinc-700 dark:text-zinc-300">{selected.secret_name}</dd>
          {/if}
          {#if selected.remote_addr}
            <dt class="text-zinc-500 dark:text-zinc-400">Remote</dt>
            <dd class="font-mono text-zinc-700 dark:text-zinc-300">{selected.remote_addr}</dd>
          {/if}
          {#if selected.client && typeof selected.client === 'object'}
            <dt class="text-zinc-500 dark:text-zinc-400">Client</dt>
            <dd class="font-mono text-zinc-700 dark:text-zinc-300">
              {JSON.stringify(selected.client)}
            </dd>
          {/if}
        </dl>
        {#if selected.details}
          <details class="mt-3">
            <summary class="cursor-pointer text-xs text-zinc-500 dark:text-zinc-400">
              Details
            </summary>
            <pre
              class="mt-2 max-h-64 select-text overflow-auto rounded bg-zinc-50 p-2 text-xs text-zinc-800 dark:bg-zinc-950 dark:text-zinc-200">{JSON.stringify(selected.details, null, 2)}</pre>
          </details>
        {/if}
        <details class="mt-3">
          <summary class="cursor-pointer text-xs text-zinc-500 dark:text-zinc-400">
            Raw JSON
          </summary>
          <pre
            class="mt-2 max-h-64 select-text overflow-auto rounded bg-zinc-50 p-2 text-xs text-zinc-800 dark:bg-zinc-950 dark:text-zinc-200">{JSON.stringify(selected, null, 2)}</pre>
        </details>
      {/if}
    </Card>
  </div>
</div>
