import { describe, expect, it } from 'vitest'
import { describeChange } from './format'
import { presetToRange } from './range'

describe('presetToRange', () => {
  it('uses the calendar day in the station timezone, not the browser', () => {
    // 22:30 UTC on Oct 17 is already Oct 18 in Vilnius (UTC+3).
    const now = new Date('2026-10-17T22:30:00Z')
    expect(presetToRange('today', 'Europe/Vilnius', now)).toEqual({
      from: '2026-10-18',
      to: '2026-10-19',
    })
    expect(presetToRange('today', 'UTC', now)).toEqual({ from: '2026-10-17', to: '2026-10-18' })
  })

  it('produces half-open multi-day ranges across DST changes', () => {
    const now = new Date('2026-10-27T10:00:00Z')
    expect(presetToRange('7d', 'Europe/Vilnius', now)).toEqual({
      from: '2026-10-21',
      to: '2026-10-28',
    })
  })
})

describe('describeChange', () => {
  it('never renders infinite growth', () => {
    expect(describeChange({ previous: 0, delta: 5, state: 'new', percent: null })).toMatch(/New/)
    expect(describeChange({ previous: 0, delta: 0, state: 'none', percent: null })).toMatch(
      /No plays/,
    )
    expect(describeChange({ previous: 4, delta: 4, state: 'up', percent: 100 })).toBe(
      '+100% vs previous period (4)',
    )
  })
})
