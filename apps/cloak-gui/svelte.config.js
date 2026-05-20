import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

/** @type {import('@sveltejs/vite-plugin-svelte').SvelteConfig} */
export default {
  preprocess: vitePreprocess(),

  compilerOptions: {
    // Svelte 5 — enable runes mode globally.
    runes: true,
  },
};
