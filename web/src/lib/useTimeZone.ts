import { useSelector } from '@tanstack/react-store'
import { useStationTimeZone } from '../api/queries'
import { browserTimeZone, prefsStore } from '../state/prefs'

/** The timezone used to display timestamps, per the user's preference. */
export function useDisplayTimeZone(): string {
  const station = useStationTimeZone()
  const mode = useSelector(prefsStore, (s) => s.timeZoneMode)
  return mode === 'browser' ? browserTimeZone() : station
}
