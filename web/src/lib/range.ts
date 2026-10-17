import { addDays, isoDateIn } from './format'

// Range presets are expressed as calendar days in the station (analytics)
// timezone and sent as YYYY-MM-DD, which the API interprets as local
// midnight there. The range is half-open: [from, to).

export const RANGE_PRESETS = ['today', '7d', '30d', '90d', '365d'] as const
export type RangePreset = (typeof RANGE_PRESETS)[number]

export const PRESET_LABELS: Record<RangePreset, string> = {
  today: 'Today',
  '7d': '7 days',
  '30d': '30 days',
  '90d': '90 days',
  '365d': '1 year',
}

const PRESET_DAYS: Record<RangePreset, number> = {
  today: 1,
  '7d': 7,
  '30d': 30,
  '90d': 90,
  '365d': 365,
}

export function isRangePreset(v: unknown): v is RangePreset {
  return typeof v === 'string' && (RANGE_PRESETS as readonly string[]).includes(v)
}

export function presetToRange(
  preset: RangePreset,
  timeZone: string,
  now: Date = new Date(),
): { from: string; to: string } {
  const today = isoDateIn(now, timeZone)
  const to = addDays(today, 1)
  return { from: addDays(to, -PRESET_DAYS[preset]), to }
}

/** A trend bucket that keeps charts readable for the selected range. */
export function bucketFor(preset: RangePreset): 'hour' | 'day' | 'week' {
  if (preset === 'today') return 'hour'
  if (preset === '365d') return 'week'
  return 'day'
}
