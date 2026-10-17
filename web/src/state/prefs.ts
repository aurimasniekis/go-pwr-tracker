import { createStore } from '@tanstack/react-store'

// Local UI preferences shared across pages. Server data lives only in the
// TanStack Query cache; this store never holds API responses.

export type TimeZoneMode = 'station' | 'browser'

export interface Prefs {
  /** Show timestamps in the station's analytics timezone or the browser's. */
  timeZoneMode: TimeZoneMode
}

const STORAGE_KEY = 'pwr-tracker:prefs'
const defaults: Prefs = { timeZoneMode: 'station' }

function load(): Prefs {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return defaults
    const parsed = JSON.parse(raw) as Partial<Prefs>
    return {
      timeZoneMode: parsed.timeZoneMode === 'browser' ? 'browser' : 'station',
    }
  } catch {
    return defaults
  }
}

export const prefsStore = createStore<Prefs>(load())

prefsStore.subscribe((value) => {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(value))
  } catch {
    // storage unavailable (private mode, blocked); preferences stay in memory
  }
})

export function setTimeZoneMode(mode: TimeZoneMode) {
  prefsStore.setState((s) => ({ ...s, timeZoneMode: mode }))
}

export const browserTimeZone = (): string =>
  Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC'
