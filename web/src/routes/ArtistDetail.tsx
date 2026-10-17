import { useQuery } from '@tanstack/react-query'
import { getRouteApi, Link } from '@tanstack/react-router'
import {
  artistQuery,
  recentPlaysForQuery,
  topTracksQuery,
  useStationTimeZone,
} from '../api/queries'
import { PlayList } from '../components/PlayList'
import { Empty, ErrorMessage, Loading } from '../components/QueryStates'
import { RangePicker } from '../components/RangePicker'
import { TopTracksList } from '../components/TopLists'
import { formatCount, formatDateTime } from '../lib/format'
import { PRESET_LABELS, presetToRange } from '../lib/range'
import { useDisplayTimeZone } from '../lib/useTimeZone'
import { EntityAnalytics } from './detailShared'

const route = getRouteApi('/artists/$artistId')

export function ArtistDetail() {
  const { artistId } = route.useParams()
  const preset = route.useSearch({ select: (s) => s.range ?? '30d' })
  const navigate = route.useNavigate()
  const station = useStationTimeZone()
  const tz = useDisplayTimeZone()
  const range = presetToRange(preset, station)
  const artist = useQuery(artistQuery(artistId))
  const top = useQuery(topTracksQuery({ ...range, artist_id: artistId, limit: 20 }))
  const recent = useQuery(recentPlaysForQuery({ artist_id: artistId }))

  if (artist.isPending) return <Loading />
  if (artist.isError) return <ErrorMessage error={artist.error} onRetry={() => artist.refetch()} />
  const a = artist.data

  return (
    <div className="detail">
      <section className="card">
        <p className="eyebrow">Artist</p>
        <h1 className="detail-title">{a.name}</h1>
        <dl className="facts">
          <div>
            <dt>Total plays</dt>
            <dd>
              {formatCount(a.total_plays)}
              {a.uncertain_plays > 0 && (
                <span className="muted"> ({a.uncertain_plays} uncertain)</span>
              )}
            </dd>
          </div>
          <div>
            <dt>Tracks observed</dt>
            <dd>{a.track_count}</dd>
          </div>
          {a.first_observed_at && (
            <div>
              <dt>First observed</dt>
              <dd>{formatDateTime(a.first_observed_at, tz)}</dd>
            </div>
          )}
        </dl>
        <p className="muted small">
          Artist names are kept exactly as the station reports them; collaborations (“A &amp; B”)
          are a separate artist entry from each individual artist.
        </p>
      </section>

      <div className="section-head">
        <h2>Airplay · {PRESET_LABELS[preset]}</h2>
        <RangePicker
          value={preset}
          onChange={(v) =>
            navigate({ search: { range: v === '30d' ? undefined : v }, replace: true })
          }
        />
      </div>

      <section className="card">
        <h2>Top tracks</h2>
        {top.isPending ? (
          <Loading />
        ) : top.isError ? (
          <ErrorMessage error={top.error} onRetry={() => top.refetch()} />
        ) : top.data.items.length === 0 ? (
          <Empty>No plays in this period.</Empty>
        ) : (
          <TopTracksList items={top.data.items} showArtist={false} />
        )}
      </section>

      <EntityAnalytics params={{ ...range, artist_id: artistId }} preset={preset} timeZone={tz} />

      <section className="card">
        <div className="card-head">
          <h2>Recent airplay</h2>
          <Link to="/activity" search={{ artist_id: artistId }} className="more-link">
            All plays →
          </Link>
        </div>
        {recent.isPending ? (
          <Loading />
        ) : recent.isError ? (
          <ErrorMessage error={recent.error} />
        ) : recent.data.items.length === 0 ? (
          <Empty>No plays recorded.</Empty>
        ) : (
          <PlayList plays={recent.data.items} timeZone={tz} showDate />
        )}
      </section>
    </div>
  )
}
