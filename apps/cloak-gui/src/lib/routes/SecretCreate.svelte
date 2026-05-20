<script lang="ts">
  import { navigate } from '$lib/router.svelte';
  import type { SecretType } from '$lib/api';
  import PostgresForm from './secret-forms/PostgresForm.svelte';
  import MySQLForm from './secret-forms/MySQLForm.svelte';
  import SSHForm from './secret-forms/SSHForm.svelte';
  import HTTPForm from './secret-forms/HTTPForm.svelte';
  import EnvForm from './secret-forms/EnvForm.svelte';

  let chosen = $state<SecretType | null>(null);

  interface Option {
    type: SecretType;
    title: string;
    description: string;
    iconPath: string;
  }

  // Mono-line icons for the type picker. Each path is centred in a 24×24 box.
  const options: Option[] = [
    {
      type: 'postgres',
      title: 'PostgreSQL',
      description: 'Wire-protocol proxy for psql, pgx, JDBC, GUI clients.',
      iconPath:
        'M5 5h14v5a7 7 0 0 1-7 7 7 7 0 0 1-7-7V5zM5 14h14v3a7 7 0 0 1-7 5 7 7 0 0 1-7-5v-3z',
    },
    {
      type: 'mysql',
      title: 'MySQL',
      description: 'Wire-protocol proxy for mysql client, drivers, GUI tools.',
      iconPath:
        'M3 12c2-3 5-5 9-5s7 2 9 5M3 12c2 3 5 5 9 5s7-2 9-5M12 7v10',
    },
    {
      type: 'ssh',
      title: 'SSH',
      description: 'Password / private-key authenticated SSH, SFTP, ProxyJump.',
      iconPath: 'M4 6l8 6-8 6V6zM13 17h7v3h-7v-3z',
    },
    {
      type: 'http',
      title: 'HTTP',
      description: 'Reverse-proxy with header / query injection.',
      iconPath: 'M3 12h18M12 3a15 15 0 0 1 0 18M12 3a15 15 0 0 0 0 18M3 12a9 9 0 0 1 18 0M3 12a9 9 0 0 0 18 0',
    },
    {
      type: 'env',
      title: 'Env',
      description: 'Inject credentials into CLI tools — AWS CLI, gcloud, kubectl.',
      iconPath: 'M4 5h16v14H4zM7 9l3 3-3 3M13 15h4',
    },
  ];

  const titles: Record<SecretType, string> = {
    postgres: 'Add PostgreSQL secret',
    mysql: 'Add MySQL secret',
    ssh: 'Add SSH secret',
    http: 'Add HTTP secret',
    env: 'Add Env secret',
  };
</script>

<div class="mx-auto flex max-w-2xl flex-col gap-6 p-8">
  <header>
    <button
      type="button"
      class="text-xs text-zinc-500 hover:underline dark:text-zinc-400"
      onclick={() => (chosen ? (chosen = null) : navigate('secrets'))}
    >
      ← {chosen ? 'Choose a different type' : 'Back to secrets'}
    </button>
    <h1 class="mt-2 text-2xl font-semibold tracking-tight text-zinc-900 dark:text-zinc-100">
      {chosen ? titles[chosen] : 'Add a secret'}
    </h1>
    <p class="text-sm text-zinc-500 dark:text-zinc-400">
      {#if !chosen}
        Pick what Cloak should manage.
      {:else if chosen === 'env'}
        Cloak stores these values encrypted and injects them into commands you run.
      {:else}
        Cloak will store these credentials encrypted and expose a local endpoint at 127.0.0.1.
      {/if}
    </p>
  </header>

  {#if !chosen}
    <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
      {#each options as opt (opt.type)}
        <button
          type="button"
          onclick={() => (chosen = opt.type)}
          class="
            flex flex-col gap-2 rounded-xl border border-zinc-200 bg-white p-5 text-left shadow-sm
            transition hover:border-zinc-400 hover:shadow-md
            dark:border-zinc-800 dark:bg-zinc-900 dark:hover:border-zinc-600
          "
        >
          <div class="flex size-9 items-center justify-center rounded-lg bg-zinc-100 text-zinc-700 dark:bg-zinc-800 dark:text-zinc-300">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" class="size-5">
              <path d={opt.iconPath} />
            </svg>
          </div>
          <div class="font-semibold text-zinc-900 dark:text-zinc-100">{opt.title}</div>
          <div class="text-xs text-zinc-500 dark:text-zinc-400">{opt.description}</div>
        </button>
      {/each}
    </div>
  {:else if chosen === 'postgres'}
    <PostgresForm />
  {:else if chosen === 'mysql'}
    <MySQLForm />
  {:else if chosen === 'ssh'}
    <SSHForm />
  {:else if chosen === 'http'}
    <HTTPForm />
  {:else if chosen === 'env'}
    <EnvForm />
  {/if}
</div>
