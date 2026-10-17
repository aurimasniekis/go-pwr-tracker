import {
  Bar,
  BarChart,
  CartesianGrid,
  Cell,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts'
import type { Airplay } from '../api/types'
import { formatBucketLabel, formatCount, formatDateTime } from '../lib/format'

interface Row {
  label: string
  start: string
  end: string
  plays: number
  uncertain: number
  partial: boolean
}

function ChartTooltip({
  active,
  payload,
  timeZone,
}: {
  active?: boolean
  payload?: ReadonlyArray<{ payload: Row }>
  timeZone: string
}) {
  const row = payload?.[0]?.payload
  if (!active || !row) return null
  return (
    <div className="chart-tooltip">
      <div className="tt-title">
        {formatDateTime(row.start, timeZone)} – {formatDateTime(row.end, timeZone)}
      </div>
      <div>
        <strong>{formatCount(row.plays)}</strong> plays
        {row.uncertain > 0 && <span className="muted"> ({row.uncertain} uncertain)</span>}
      </div>
      {row.partial && (
        <div className="muted">Partial bucket (in progress or not fully collected)</div>
      )}
    </div>
  )
}

/**
 * Airplay over time: one series, so no legend; partial buckets use a lighter
 * step of the same hue and are called out in the tooltip and table.
 */
export function AirplayChart({ data, title }: { data: Airplay; title: string }) {
  const tz = data.range.timezone
  const rows: Row[] = data.points.map((p) => ({
    label: formatBucketLabel(p.start, data.bucket, tz),
    start: p.start,
    end: p.end,
    plays: p.plays,
    uncertain: p.uncertain_plays,
    partial: p.partial,
  }))
  const total = rows.reduce((n, r) => n + r.plays, 0)
  return (
    <figure className="chart">
      <figcaption>
        <span className="chart-title">{title}</span>
        <span className="muted">
          {' '}
          · per {data.bucket} · {tz}
        </span>
      </figcaption>
      {total === 0 ? (
        <p className="state state-empty">No plays recorded in this period.</p>
      ) : (
        <div
          className="chart-box"
          role="img"
          aria-label={`${title}: ${total} plays in ${rows.length} ${data.bucket} buckets`}
        >
          <ResponsiveContainer width="100%" height={220}>
            <BarChart
              data={rows}
              margin={{ top: 8, right: 8, bottom: 0, left: 0 }}
              barCategoryGap={2}
            >
              <CartesianGrid vertical={false} stroke="var(--grid)" />
              <XAxis
                dataKey="label"
                tick={{ fill: 'var(--text-muted)', fontSize: 12 }}
                tickLine={false}
                axisLine={{ stroke: 'var(--axis)' }}
                minTickGap={24}
              />
              <YAxis
                allowDecimals={false}
                width={36}
                tick={{ fill: 'var(--text-muted)', fontSize: 12 }}
                tickLine={false}
                axisLine={false}
              />
              <Tooltip content={<ChartTooltip timeZone={tz} />} cursor={{ fill: 'var(--hover)' }} />
              <Bar dataKey="plays" radius={[4, 4, 0, 0]} isAnimationActive={false}>
                {rows.map((r) => (
                  <Cell
                    key={r.start}
                    fill={r.partial ? 'var(--series-1-soft)' : 'var(--series-1)'}
                  />
                ))}
              </Bar>
            </BarChart>
          </ResponsiveContainer>
        </div>
      )}
      <details className="data-table">
        <summary>Data table</summary>
        <table>
          <thead>
            <tr>
              <th scope="col">Bucket start ({tz})</th>
              <th scope="col">Plays</th>
              <th scope="col">Uncertain</th>
              <th scope="col">Partial</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((r) => (
              <tr key={r.start}>
                <td>{formatDateTime(r.start, tz)}</td>
                <td>{r.plays}</td>
                <td>{r.uncertain}</td>
                <td>{r.partial ? 'yes' : ''}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </details>
    </figure>
  )
}
