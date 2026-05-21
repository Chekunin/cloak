<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { listen, type UnlistenFn } from '@tauri-apps/api/event';

  import { router, navigate } from '$lib/router.svelte';
  import { connection } from '$lib/stores/connection.svelte';
  import { vaultStore } from '$lib/stores/vault.svelte';
  import { theme } from '$lib/stores/theme.svelte';
  import { update } from '$lib/stores/update.svelte';

  import Sidebar from '$lib/components/Sidebar.svelte';
  import ConnectionBanner from '$lib/components/ConnectionBanner.svelte';
  import Toasts from '$lib/components/Toasts.svelte';
  import CommandPalette from '$lib/components/CommandPalette.svelte';
  import UpdateDialog from '$lib/components/UpdateDialog.svelte';
  import { installKeyboardShortcuts } from '$lib/keyboard.svelte';

  import Welcome from '$lib/routes/Welcome.svelte';
  import Unlock from '$lib/routes/Unlock.svelte';
  import Dashboard from '$lib/routes/Dashboard.svelte';
  import SecretCreate from '$lib/routes/SecretCreate.svelte';
  import SecretRotate from '$lib/routes/SecretRotate.svelte';
  import Run from '$lib/routes/Run.svelte';
  import Tokens from '$lib/routes/Tokens.svelte';
  import Audit from '$lib/routes/Audit.svelte';

  let uninstallShortcuts: (() => void) | null = null;
  let unlistenUpdate: UnlistenFn | null = null;

  onMount(() => {
    theme.init();
    connection.start();
    vaultStore.start();
    uninstallShortcuts = installKeyboardShortcuts();
    // The tray's "Check for Updates…" item emits this; run the check here.
    void listen('menu://check-update', () => void update.check()).then((fn) => {
      unlistenUpdate = fn;
    });
  });

  onDestroy(() => {
    connection.stop();
    vaultStore.stop();
    uninstallShortcuts?.();
    unlistenUpdate?.();
  });

  /**
   * Route guard: redirect based on vault state.
   *
   * - Uninitialized → force `welcome`.
   * - Locked        → force `unlock` unless already there.
   * - Unlocked      → if user landed on `welcome`/`unlock`, go to `dashboard`.
   *
   * Skipped until the first successful vault-status poll so we don't bounce
   * the user before we know what state we're in.
   */
  $effect(() => {
    if (vaultStore.phase.kind !== 'ok') return;
    const state = vaultStore.phase.status.state;
    const current = router.route.path;

    if (state === 'uninitialized' && current !== 'welcome') {
      navigate('welcome');
    } else if (state === 'locked' && current !== 'unlock' && current !== 'welcome') {
      navigate('unlock');
    } else if (state === 'unlocked' && (current === 'welcome' || current === 'unlock')) {
      navigate('dashboard');
    }
  });

  // Setup screens (welcome / unlock) are full-screen — no sidebar.
  const showShell = $derived(
    router.route.path !== 'welcome' && router.route.path !== 'unlock',
  );
</script>

<div class="flex h-full flex-col">
  {#if showShell}
    <div class="flex h-full">
      <Sidebar />
      <div class="flex h-full flex-1 flex-col overflow-hidden">
        <ConnectionBanner />
        <main class="flex-1 overflow-y-auto">
          {#if router.route.path === 'dashboard'}
            <Dashboard />
          {:else if router.route.path === 'secrets:create'}
            <SecretCreate />
          {:else if router.route.path === 'secrets:rotate'}
            {#key router.route.params[0]}
              <SecretRotate />
            {/key}
          {:else if router.route.path === 'run'}
            <Run />
          {:else if router.route.path === 'tokens'}
            <Tokens />
          {:else if router.route.path === 'audit'}
            <Audit />
          {:else}
            <Dashboard />
          {/if}
        </main>
      </div>
    </div>
  {:else}
    <main class="flex h-full items-center justify-center">
      {#if router.route.path === 'welcome'}
        <Welcome />
      {:else}
        <Unlock />
      {/if}
    </main>
  {/if}

  <CommandPalette />
  <UpdateDialog />
  <Toasts />
</div>
