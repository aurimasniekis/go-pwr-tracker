import { screen, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { collector, nowPlaying, play, playsPage, summary } from '../test/fixtures'
import { renderApp } from '../test/render'

const emptyRanking = { range: summary().range, previous: null, items: [] }
const emptyAirplay = { range: summary().range, bucket: 'day', points: [] }

function api(overrides: Record<string, { status?: number; body: unknown }>) {
  return (url: URL) => {
    const key = url.pathname.replace('/api/v1', '')
    if (overrides[key]) return overrides[key]
    switch (key) {
      case '/now-playing':
        return { body: nowPlaying() }
      case '/plays':
        return { body: playsPage([]) }
      case '/stats/summary':
        return { body: summary(0) }
      case '/stats/top-tracks':
      case '/stats/top-artists':
        return { body: emptyRanking }
      case '/stats/airplay':
        return { body: emptyAirplay }
    }
    return undefined
  }
}

afterEach(() => vi.unstubAllGlobals())

describe('Dashboard', () => {
  it('shows now playing as text and live status', async () => {
    renderApp('/', api({}))
    const card = await screen.findByRole('region', { name: 'Now playing' })
    expect(within(card).getByText('Sustoja Laikas (Meile)')).toBeInTheDocument()
    expect(within(card).getByText('Leon Somov & Kaia')).toBeInTheDocument()
    expect(within(card).getByText(/Live/)).toBeInTheDocument()
  })

  it('renders upstream values as text, never HTML', async () => {
    const evil = play({
      track: { id: 1, title: '<img src=x onerror=alert(1)>', kind: 'music', artist: null },
    })
    const { container } = renderApp(
      '/',
      api({ '/now-playing': { body: nowPlaying({ play: evil }) } }),
    )
    expect(await screen.findAllByText('<img src=x onerror=alert(1)>')).not.toHaveLength(0)
    expect(container.querySelector('img')).toBeNull()
  })

  it('keeps the last value but flags stale data and the last error', async () => {
    const stale = nowPlaying({
      collector: collector({
        state: 'stale',
        stale: true,
        last_success_at: new Date(Date.now() - 20 * 60_000).toISOString(),
        consecutive_failures: 4,
        last_error: 'upstream_status: upstream returned HTTP 503',
      }),
    })
    renderApp('/', api({ '/now-playing': { body: stale } }))
    const card = await screen.findByRole('region', { name: 'Last seen on air' })
    expect(within(card).getByText('Sustoja Laikas (Meile)')).toBeInTheDocument()
    expect(within(card).getByText(/Data may be out of date/)).toBeInTheDocument()
    expect(within(card).getByText(/HTTP 503/)).toBeInTheDocument()
    expect(within(card).getByText(/20 min ago/)).toBeInTheDocument()
  })

  it('shows empty states before any data is collected', async () => {
    const starting = nowPlaying({
      play: null,
      collector: collector({ state: 'starting', stale: true, last_success_at: null }),
    })
    renderApp('/', api({ '/now-playing': { body: starting } }))
    expect(await screen.findByText(/Nothing observed yet/)).toBeInTheDocument()
    expect(await screen.findByText('No plays recorded yet.')).toBeInTheDocument()
    expect(await screen.findByText('No plays recorded in this period.')).toBeInTheDocument()
    expect(screen.getAllByText('No plays in this period.')).toHaveLength(2)
  })

  it('shows API errors with a retry action', async () => {
    renderApp(
      '/',
      api({
        '/plays': {
          status: 500,
          body: { error: { code: 'internal', message: 'internal server error' } },
        },
      }),
    )
    const alert = await screen.findByRole('alert')
    expect(alert).toHaveTextContent('internal server error')
    expect(within(alert).getByRole('button', { name: 'Retry' })).toBeInTheDocument()
  })

  it('marks inferred and uncertain plays in the recent list', async () => {
    const inferred = play({
      id: 2,
      start_authoritative: false,
      upstream_start_at: null,
      reason: 'continuity_gap',
      uncertain: true,
    })
    renderApp('/', api({ '/plays': { body: playsPage([inferred]) } }))
    expect(await screen.findByText('first seen')).toBeInTheDocument()
    expect(screen.getByText('uncertain')).toBeInTheDocument()
  })

  it('shows "New" instead of an infinite percentage when the previous period had no plays', async () => {
    renderApp('/', api({ '/stats/summary': { body: summary(12) } }))
    expect(await screen.findByLabelText('New: no plays in the previous period')).toHaveTextContent(
      'New',
    )
  })
})
