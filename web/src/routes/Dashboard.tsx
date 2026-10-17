import { useQuery } from '@tanstack/react-query'
import { getRouteApi, Link } from '@tanstack/react-router'
import {
  airplayQuery,
  recentPlaysQuery,
  summaryQuery,
  topArtistsQuery,
  topTracksQuery,
  useNowPlaying,
  useStationTimeZone,
} from '../api/queries'
import { AirplayChart } from '../components/AirplayChart'
import { NowPlayingCard } from '../components/NowPlayingCard'
import { PlayList } from '../components/PlayList'
import { Empty, ErrorMessage, Loading } from '../components/QueryStates'
import { RangePicker } from '../components/RangePicker'
import { CoverageNote, SummaryTiles } from '../components/SummaryTiles'
import { TopArtistsList, TopTracksList } from '../components/TopLists'
import { bucketFor, PRESET_LABELS, presetToRange } from '../lib/range'
import { useDisplayTimeZone } from '../lib/useTimeZone'

const route = getRouteApi('/')

export function Dashboard() {
  const preset = route.useSearch({ select: (s) => s.range ?? '7d' })
  const navigate = route.useNavigate()
  const station = useStationTimeZone()
  const tz = useDisplayTimeZone()
  const range = presetToRange(preset, station)

  const np = useNowPlaying()
  const recent = useQuery(recentPlaysQuery(10))
  const summary = useQuery(summaryQuery(range))
  const topTracks = useQuery(topTracksQuery({ ...range, limit: 10 }))
  const topArtists = useQuery(topArtistsQuery({ ...range, limit: 10 }))
  const trend = useQuery(airplayQuery({ ...range, bucket: bucketFor(preset) }))

  return (
    <div className="dashboard">
      <div className="grid-2">
        {np.isPending ? (
          <section className="card">
            <Loading label="Loading now playing…" />
          </section>
        ) : np.isError ? (
          <section className="card">
            <h2>Now playing</h2>
            <ErrorMessage error={np.error} onRetry={() => np.refetch()} />
          </section>
        ) : (
          <NowPlayingCard data={np.data} timeZone={tz} />
        )}

        <section className="card" aria-labelledby="recent-h">
          <div className="card-head">
            <h2 id="recent-h">Recent plays</h2>
            <Link to="/activity" className="more-link">
              All activity →
            </Link>
          </div>
          {recent.isPending ? (
            <Loading />
          ) : recent.isError ? (
            <ErrorMessage error={recent.error} onRetry={() => recent.refetch()} />
          ) : recent.data.items.length === 0 ? (
            <Empty>No plays recorded yet.</Empty>
          ) : (
            <PlayList plays={recent.data.items} timeZone={tz} />
          )}
        </section>
      </div>

      <div className="section-head">
        <h2>Airplay · {PRESET_LABELS[preset]}</h2>
        <RangePicker
          value={preset}
          onChange={(v) =>
            navigate({ search: { range: v === '7d' ? undefined : v }, replace: true })
          }
        />
      </div>

      {summary.isPending ? (
        <Loading />
      ) : summary.isError ? (
        <ErrorMessage error={summary.error} onRetry={() => summary.refetch()} />
      ) : (
        <>
          <SummaryTiles data={summary.data} />
          <CoverageNote data={summary.data} timeZone={tz} />
        </>
      )}

      <section className="card">
        {trend.isPending ? (
          <Loading />
        ) : trend.isError ? (
          <ErrorMessage error={trend.error} onRetry={() => trend.refetch()} />
        ) : (
          <AirplayChart data={trend.data} title="Plays over time" />
        )}
      </section>

      <div className="grid-2">
        <section className="card" aria-labelledby="top-tracks-h">
          <h2 id="top-tracks-h">Most played tracks</h2>
          {topTracks.isPending ? (
            <Loading />
          ) : topTracks.isError ? (
            <ErrorMessage error={topTracks.error} onRetry={() => topTracks.refetch()} />
          ) : topTracks.data.items.length === 0 ? (
            <Empty>No plays in this period.</Empty>
          ) : (
            <TopTracksList items={topTracks.data.items} />
          )}
        </section>
        <section className="card" aria-labelledby="top-artists-h">
          <h2 id="top-artists-h">Most played artists</h2>
          {topArtists.isPending ? (
            <Loading />
          ) : topArtists.isError ? (
            <ErrorMessage error={topArtists.error} onRetry={() => topArtists.refetch()} />
          ) : topArtists.data.items.length === 0 ? (
            <Empty>No plays in this period.</Empty>
          ) : (
            <TopArtistsList items={topArtists.data.items} />
          )}
        </section>
      </div>
    </div>
  )
}
