import { useQuery } from '@tanstack/react-query'
import { getRouteApi, Link } from '@tanstack/react-router'
import { recentPlaysForQuery, trackQuery, useStationTimeZone } from '../api/queries'
import { PlayList } from '../components/PlayList'
import { Empty, ErrorMessage, Loading } from '../components/QueryStates'
import { RangePicker } from '../components/RangePicker'
import { formatCount, formatDateTime } from '../lib/format'
import { PRESET_LABELS, presetToRange } from '../lib/range'
import { useDisplayTimeZone } from '../lib/useTimeZone'
import { EntityAnalytics } from './detailShared'

const route = getRouteApi('/tracks/$trackId')

export function TrackDetail() {
  const { trackId } = route.useParams()
  const preset = route.useSearch({ select: (s) => s.range ?? '30d' })
  const navigate = route.useNavigate()
  const station = useStationTimeZone()
  const tz = useDisplayTimeZone()
  const track = useQuery(trackQuery(trackId))
  const recent = useQuery(recentPlaysForQuery({ track_id: trackId }))

  if (track.isPending) return <Loading />
  if (track.isError) return <ErrorMessage error={track.error} onRetry={() => track.refetch()} />
  const t = track.data
  const range = presetToRange(preset, station)

  return (
    <div className="detail">
      <section className="card">
        <p className="eyebrow">Track</p>
        <h1 className="detail-title">{t.title}</h1>
        {t.artist && (
          <p className="detail-sub">
            by{' '}
            <Link to="/artists/$artistId" params={{ artistId: t.artist.id }}>
              {t.artist.name}
            </Link>
          </p>
        )}
        <dl className="facts">
          <div>
            <dt>Total plays</dt>
            <dd>
              {formatCount(t.total_plays)}
              {t.uncertain_plays > 0 && (
                <span className="muted"> ({t.uncertain_plays} uncertain)</span>
              )}
            </dd>
          </div>
          <div>
            <dt>First observed</dt>
            <dd>{formatDateTime(t.first_observed_at, tz)}</dd>
          </div>
          <div>
            <dt>Last observed</dt>
            <dd>{formatDateTime(t.last_observed_at, tz)}</dd>
          </div>
        </dl>
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
      <EntityAnalytics
        params={{ ...range, track_id: trackId }}
        preset={preset}
        timeZone={tz}
        showDistinctTiles={false}
      />

      <section className="card">
        <div className="card-head">
          <h2>Recent plays</h2>
          <Link to="/activity" search={{ track_id: trackId }} className="more-link">
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
          <PlayList plays={recent.data.items} timeZone={tz} showDate hideTrack />
        )}
      </section>
    </div>
  )
}
