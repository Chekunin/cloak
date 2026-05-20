/**
 * Theme: system / light / dark.
 *
 * Persists the preference to localStorage and reflects it on the document
 * root via Tailwind v4's `dark` class. Listens for OS-level changes when
 * the preference is `system`.
 */

export type ThemePreference = 'system' | 'light' | 'dark';
export type ResolvedTheme = 'light' | 'dark';

const STORAGE_KEY = 'cloak.theme';

class ThemeStore {
  preference: ThemePreference = $state(loadPreference());
  resolved: ResolvedTheme = $state('light');

  private media: MediaQueryList | null = null;

  init(): void {
    if (typeof window === 'undefined') return;
    this.media = window.matchMedia('(prefers-color-scheme: dark)');
    this.media.addEventListener('change', () => this.apply());
    this.apply();
  }

  set(pref: ThemePreference): void {
    this.preference = pref;
    try {
      localStorage.setItem(STORAGE_KEY, pref);
    } catch {
      // localStorage unavailable — non-fatal, theme just doesn't persist.
    }
    this.apply();
  }

  private apply(): void {
    if (typeof document === 'undefined') return;
    const wantDark =
      this.preference === 'dark' ||
      (this.preference === 'system' && (this.media?.matches ?? false));
    this.resolved = wantDark ? 'dark' : 'light';
    document.documentElement.classList.toggle('dark', wantDark);
  }
}

function loadPreference(): ThemePreference {
  if (typeof localStorage === 'undefined') return 'system';
  const v = localStorage.getItem(STORAGE_KEY);
  if (v === 'light' || v === 'dark' || v === 'system') return v;
  return 'system';
}

export const theme = new ThemeStore();
