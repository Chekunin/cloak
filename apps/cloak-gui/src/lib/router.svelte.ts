/**
 * Tiny rune-based hash router.
 *
 * Routes are identified by a string like "dashboard" or "secrets:create";
 * any extra `:`-separated segments after the matched path are exposed via
 * `params`. For example, `#secrets:edit:prod-db` resolves to
 * `{ path: "secrets:edit", params: ["prod-db"] }`.
 *
 * Usage:
 *   import { router, navigate } from '$lib/router.svelte';
 *   router.route.path                  // → "dashboard"
 *   router.route.params[0]             // → "prod-db" for parametric routes
 *   navigate('secrets:edit', 'prod-db')
 *
 * Why not svelte-spa-router? A handful of named screens, no deep linking,
 * no auth-aware redirects. Bespoke routing is easier to audit and ~80 lines.
 */

export type RoutePath =
  | 'welcome'
  | 'unlock'
  | 'dashboard'
  | 'secrets:create'
  | 'secrets:edit'
  | 'secrets:rotate'
  | 'run'
  | 'tokens'
  | 'audit';

/** Two-segment routes — must be matched before falling through to the prefix. */
const TWO_SEGMENT_ROUTES = new Set<RoutePath>([
  'secrets:create',
  'secrets:edit',
  'secrets:rotate',
]);

const ONE_SEGMENT_ROUTES = new Set<RoutePath>([
  'welcome',
  'unlock',
  'dashboard',
  'run',
  'tokens',
  'audit',
]);

interface Route {
  path: RoutePath;
  params: string[];
}

class Router {
  /** Current parsed route. Reactive — components read this directly. */
  route: Route = $state(parseHash());

  constructor() {
    if (typeof window !== 'undefined') {
      window.addEventListener('hashchange', () => {
        this.route = parseHash();
      });
    }
  }

  navigate(path: RoutePath, ...params: string[]): void {
    const segments = [path, ...params].join(':');
    window.location.hash = `#${segments}`;
  }

  is(path: RoutePath): boolean {
    return this.route.path === path;
  }

  /** True when the current route's primary path is `prefix` (e.g. all `secrets:*` count under `secrets`). */
  isPrefix(prefix: RoutePath): boolean {
    return this.route.path === prefix || this.route.path.startsWith(`${prefix}:`);
  }
}

function parseHash(): Route {
  if (typeof window === 'undefined') {
    return { path: 'welcome', params: [] };
  }
  const raw = window.location.hash.replace(/^#/, '');
  if (!raw) return { path: 'welcome', params: [] };
  const parts = raw.split(':');

  // Greedy: try longest known match first.
  if (parts.length >= 2) {
    const two = `${parts[0]}:${parts[1]}` as RoutePath;
    if (TWO_SEGMENT_ROUTES.has(two)) {
      return { path: two, params: parts.slice(2) };
    }
  }
  const one = parts[0] as RoutePath;
  if (ONE_SEGMENT_ROUTES.has(one)) {
    return { path: one, params: parts.slice(1) };
  }
  return { path: 'welcome', params: [] };
}

export const router = new Router();

export function navigate(path: RoutePath, ...params: string[]): void {
  router.navigate(path, ...params);
}
