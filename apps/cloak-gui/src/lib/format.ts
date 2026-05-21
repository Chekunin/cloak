/**
 * Display formatting helpers — single source of truth so the same date /
 * duration / byte-count looks the same across screens.
 */

/**
 * Parse a wire timestamp, treating unparseable input and sentinel "never"
 * values (Go's 0001-01-01 zero time, the Unix epoch) as absent.
 */
function parseTime(iso: string | null | undefined): Date | null {
  if (!iso) return null;
  const t = new Date(iso);
  if (Number.isNaN(t.getTime()) || t.getUTCFullYear() <= 1970) return null;
  return t;
}

/** Whether a wire timestamp carries a real value (not null / zero / garbage). */
export function isMeaningfulTime(iso: string | null | undefined): boolean {
  return parseTime(iso) !== null;
}

export function formatDate(iso: string | null | undefined): string {
  const t = parseTime(iso);
  return t ? t.toLocaleString() : '—';
}

export function formatDateOnly(iso: string | null | undefined): string {
  const t = parseTime(iso);
  return t ? t.toLocaleDateString() : '—';
}

/** "5s ago", "12m ago", "2h ago", "3d ago" — coarse-grained, good enough for lists. */
export function timeAgo(iso: string | null | undefined): string {
  const t = parseTime(iso);
  if (!t) return '—';
  const seconds = Math.floor((Date.now() - t.getTime()) / 1000);
  if (seconds < 5) return 'just now';
  if (seconds < 60) return `${seconds}s ago`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  if (days < 30) return `${days}d ago`;
  return t.toLocaleDateString();
}

/** "in 45s", "in 12m", "in 2h", "in 3d" — for future timestamps like session TTLs. */
export function timeUntil(iso: string | null | undefined): string {
  const t = parseTime(iso);
  if (!t) return '—';
  const seconds = Math.floor((t.getTime() - Date.now()) / 1000);
  if (seconds <= 0) return 'expired';
  if (seconds < 60) return `in ${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `in ${minutes}m`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `in ${hours}h`;
  const days = Math.floor(hours / 24);
  return `in ${days}d`;
}

export function formatTimeout(seconds: number): string {
  if (seconds >= 3600) return `${Math.round(seconds / 3600)}h`;
  if (seconds >= 60) return `${Math.round(seconds / 60)}m`;
  return `${seconds}s`;
}

export function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KiB`;
  if (n < 1024 * 1024 * 1024) return `${(n / (1024 * 1024)).toFixed(1)} MiB`;
  return `${(n / (1024 * 1024 * 1024)).toFixed(2)} GiB`;
}
