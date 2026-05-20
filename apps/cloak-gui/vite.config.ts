import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import tailwindcss from '@tailwindcss/vite';
import { resolve } from 'node:path';

// Tauri opens the window on this URL during development and prevents Vite's
// dynamic dev-server-port behaviour from breaking the integration.
const TAURI_DEV_PORT = 1420;

export default defineConfig(({ command }) => ({
  plugins: [svelte(), tailwindcss()],

  resolve: {
    alias: {
      $lib: resolve(__dirname, 'src/lib'),
    },
  },

  // Quiet hot-reload churn in the Tauri webview.
  clearScreen: false,

  server: {
    port: TAURI_DEV_PORT,
    strictPort: true,
    host: '127.0.0.1',
    // The Tauri webview connects over the host:port pair; HMR over the same
    // port keeps the connection simple.
    hmr: {
      protocol: 'ws',
      host: '127.0.0.1',
      port: TAURI_DEV_PORT,
    },
    watch: {
      // Don't pick up Rust source changes — `cargo` watches them.
      ignored: ['**/src-tauri/**'],
    },
  },

  build: {
    target: 'esnext',
    sourcemap: command === 'serve',
    // Tauri reads from the configured `frontendDist`.
    outDir: 'dist',
    emptyOutDir: true,
  },

  // Useful for diagnostics; the env var is set by `tauri dev`.
  envPrefix: ['VITE_', 'TAURI_ENV_'],
}));
