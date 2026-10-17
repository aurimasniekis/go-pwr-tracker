import { Link, Outlet } from '@tanstack/react-router'
import { useSelector } from '@tanstack/react-store'
import { useNowPlaying, useStationTimeZone } from '../api/queries'
import { StatusBadge } from '../components/StatusBadge'
import { browserTimeZone, prefsStore, setTimeZoneMode } from '../state/prefs'

export function RootLayout() {
  const np = useNowPlaying()
  const station = useStationTimeZone()
  const mode = useSelector(prefsStore, (s) => s.timeZoneMode)
  const browser = browserTimeZone()
  return (
    <div className="app">
      <a href="#main" className="skip-link">
        Skip to content
      </a>
      <header className="app-header">
        <Link to="/" className="brand">
          <span className="brand-mark" aria-hidden="true" />
          pwr-tracker
        </Link>
        <nav aria-label="Main">
          <Link to="/" activeOptions={{ exact: true, includeSearch: false }}>
            Dashboard
          </Link>
          <Link to="/activity">Activity</Link>
          <Link to="/search">Search</Link>
        </nav>
        <div className="header-status">{np.data && <StatusBadge status={np.data.collector} />}</div>
      </header>
      <main id="main" className="app-main">
        <Outlet />
      </main>
      <footer className="app-footer">
        <div className="tz-pref">
          <span id="tz-label">
            Times shown in <strong>{mode === 'browser' ? browser : station}</strong>
          </span>
          {browser !== station && (
            <div className="segmented segmented-small" role="group" aria-labelledby="tz-label">
              <button
                type="button"
                aria-pressed={mode === 'station'}
                className={mode === 'station' ? 'active' : undefined}
                onClick={() => setTimeZoneMode('station')}
              >
                Station time
              </button>
              <button
                type="button"
                aria-pressed={mode === 'browser'}
                className={mode === 'browser' ? 'active' : undefined}
                onClick={() => setTimeZoneMode('browser')}
              >
                My time
              </button>
            </div>
          )}
        </div>
        <p className="muted">
          Airplay as observed by polling the station&apos;s now-playing feed. Rankings reflect how
          often tracks were played, not listener popularity. Charts and buckets always use station
          time ({station}).{' '}
          <a href="https://github.com/aurimasniekis/go-pwr-tracker" rel="noreferrer">
            Source
          </a>
        </p>
      </footer>
    </div>
  )
}

export function NotFound() {
  return (
    <section className="card">
      <h1>Page not found</h1>
      <p>
        <Link to="/">Back to the dashboard</Link>
      </p>
    </section>
  )
}
