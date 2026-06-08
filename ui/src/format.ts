// Resource formatting helpers. CPU is in millicores, memory in bytes.

export function cores(milli: number): string {
  return milli % 1000 === 0 ? String(milli / 1000) : (milli / 1000).toFixed(1);
}

export function millis(milli: number): string {
  return `${Math.round(milli)}m`;
}

export function gib(bytes: number): string {
  return (bytes / 1024 ** 3).toFixed(1);
}

export function mib(bytes: number): string {
  return `${Math.round(bytes / 1024 ** 2)}Mi`;
}

export function pct(used: number, total: number): number {
  if (total <= 0) return 0;
  return Math.min(100, Math.round((used / total) * 100));
}
