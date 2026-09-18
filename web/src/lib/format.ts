const UNITS = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']

export function fmtBytes(n: number): string {
  if (!isFinite(n) || n < 0) return '-'
  if (n < 1024) return `${Math.round(n)} B`
  let i = 0
  while (n >= 1024 && i < UNITS.length - 1) {
    n /= 1024
    i++
  }
  return `${n.toFixed(n >= 100 ? 0 : n >= 10 ? 1 : 2)} ${UNITS[i]}`
}

export function fmtRate(n: number): string {
  return fmtBytes(n) + '/s'
}

export function fmtDur(ms: number): string {
  if (ms < 0 || !isFinite(ms)) return '-'
  const s = Math.floor(ms / 1000)
  if (s < 60) return `${s}s`
  const m = Math.floor(s / 60)
  if (m < 60) return `${m}m${s % 60}s`
  const h = Math.floor(m / 60)
  if (h < 24) return `${h}h${m % 60}m`
  return `${Math.floor(h / 24)}d${h % 24}h`
}

export function fmtUptime(sec: number): string {
  return fmtDur(sec * 1000)
}

export function fmtTime(unixMs: number): string {
  const d = new Date(unixMs)
  const p = (n: number) => String(n).padStart(2, '0')
  return `${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`
}
