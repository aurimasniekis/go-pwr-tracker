import type { Bucket, Change } from '../api/types'

const cache = new Map<string, Intl.DateTimeFormat>()

function fmt(timeZone: string, opts: Intl.DateTimeFormatOptions): Intl.DateTimeFormat {
  const key = timeZone + JSON.stringify(opts)
  let f = cache.get(key)
  if (!f) {
    f = new Intl.DateTimeFormat('en-GB', { timeZone, ...opts })
    cache.set(key, f)
  }
  return f
}

export function formatDateTime(ts: string, timeZone: string): string {
  return fmt(timeZone, {
    year: 'numeric',
    month: 'short',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(ts))
}

export function formatTime(ts: string, timeZone: string): string {
  return fmt(timeZone, { hour: '2-digit', minute: '2-digit' }).format(new Date(ts))
}

export function formatDate(ts: string, timeZone: string): string {
  return fmt(timeZone, { year: 'numeric', month: 'short', day: '2-digit' }).format(new Date(ts))
}

/** Calendar date (YYYY-MM-DD) of an instant in timeZone. */
export function isoDateIn(date: Date, timeZone: string): string {
  // en-CA formats as YYYY-MM-DD.
  return new Intl.DateTimeFormat('en-CA', {
    timeZone,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
  }).format(date)
}

/** Adds days to a YYYY-MM-DD calendar date. */
export function addDays(isoDate: string, days: number): string {
  const d = new Date(`${isoDate}T00:00:00Z`)
  d.setUTCDate(d.getUTCDate() + days)
  return d.toISOString().slice(0, 10)
}

export function formatRelative(ts: string, now: Date = new Date()): string {
  const secs = Math.round((now.getTime() - new Date(ts).getTime()) / 1000)
  if (secs < 0) return 'in the future'
  if (secs < 60) return `${secs}s ago`
  const mins = Math.floor(secs / 60)
  if (mins < 60) return `${mins} min ago`
  const hours = Math.floor(mins / 60)
  if (hours < 48) return `${hours} h ago`
  return `${Math.floor(hours / 24)} days ago`
}

export function formatBucketLabel(ts: string, bucket: Bucket, timeZone: string): string {
  if (bucket === 'hour') {
    return fmt(timeZone, {
      day: '2-digit',
      month: 'short',
      hour: '2-digit',
      minute: '2-digit',
    }).format(new Date(ts))
  }
  return fmt(timeZone, { day: '2-digit', month: 'short' }).format(new Date(ts))
}

export function formatDuration(seconds: number): string {
  if (seconds < 120) return `${seconds} s`
  if (seconds < 7200) return `${Math.round(seconds / 60)} min`
  return `${Math.round(seconds / 3600)} h`
}

export function formatCount(n: number): string {
  return new Intl.NumberFormat('en-GB').format(n)
}

export function describeChange(c: Change): string {
  switch (c.state) {
    case 'up':
    case 'down':
    case 'flat':
      return `${c.percent! > 0 ? '+' : ''}${c.percent}% vs previous period (${c.previous})`
    case 'new':
      return 'New: no plays in the previous period'
    case 'none':
      return 'No plays in either period'
    case 'unavailable':
      return 'Not comparable: collection had not started for the whole previous period'
  }
}

export const WEEKDAYS = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'] as const
