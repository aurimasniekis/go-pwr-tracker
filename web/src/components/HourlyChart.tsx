import { Bar, BarChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import type { Hourly } from '../api/types'
import { WEEKDAYS } from '../lib/format'

const hourLabel = (h: number) => `${String(h).padStart(2, '0')}:00`

function HourTooltip({
  active,
  payload,
}: {
  active?: boolean
  payload?: ReadonlyArray<{ payload: { hour: number; plays: number } }>
}) {
  const row = payload?.[0]?.payload
  if (!active || !row) return null
  return (
    <div className="chart-tooltip">
      <div className="tt-title">
        {hourLabel(row.hour)}–{hourLabel((row.hour + 1) % 24)}
      </div>
      <strong>{row.plays}</strong> plays
    </div>
  )
}

export function HourlyChart({ data }: { data: Hourly }) {
  const rows = data.hours.map((plays, hour) => ({
    hour,
    label: String(hour).padStart(2, '0'),
    plays,
  }))
  return (
    <figure className="chart">
      <figcaption>
        <span className="chart-title">Plays by hour of day</span>
        <span className="muted"> · {data.range.timezone} local time</span>
      </figcaption>
      {data.plays === 0 ? (
        <p className="state state-empty">No plays recorded in this period.</p>
      ) : (
        <div
          className="chart-box"
          role="img"
          aria-label={`Plays by hour of day, ${data.plays} plays total`}
        >
          <ResponsiveContainer width="100%" height={180}>
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
                interval={2}
              />
              <YAxis
                allowDecimals={false}
                width={36}
                tick={{ fill: 'var(--text-muted)', fontSize: 12 }}
                tickLine={false}
                axisLine={false}
              />
              <Tooltip content={<HourTooltip />} cursor={{ fill: 'var(--hover)' }} />
              <Bar
                dataKey="plays"
                fill="var(--series-1)"
                radius={[4, 4, 0, 0]}
                isAnimationActive={false}
              />
            </BarChart>
          </ResponsiveContainer>
        </div>
      )}
    </figure>
  )
}

const LEVELS = 6

/** Weekday × hour heatmap on a single-hue sequential ramp. */
export function WeekdayHeatmap({ data }: { data: Hourly }) {
  const max = Math.max(0, ...data.weekday_hours.flat())
  if (data.plays === 0) return null
  return (
    <figure className="chart">
      <figcaption>
        <span className="chart-title">Weekday × hour</span>
        <span className="muted"> · darker = more plays (max {max} in one cell)</span>
      </figcaption>
      <div className="heatmap" role="table" aria-label="Plays by weekday and hour">
        <div role="row" className="heatmap-row heatmap-head">
          <span role="columnheader" />
          {Array.from({ length: 24 }, (_, h) => (
            <span role="columnheader" key={h} className="heatmap-hour">
              {h % 3 === 0 ? String(h).padStart(2, '0') : ''}
            </span>
          ))}
        </div>
        {data.weekday_hours.map((hours, d) => (
          <div role="row" className="heatmap-row" key={d}>
            <span role="rowheader" className="heatmap-day">
              {WEEKDAYS[d]}
            </span>
            {hours.map((n, h) => {
              const level = n === 0 || max === 0 ? 0 : Math.max(1, Math.ceil((n / max) * LEVELS))
              const label = `${WEEKDAYS[d]} ${hourLabel(h)}: ${n} plays`
              return (
                <span
                  role="cell"
                  key={h}
                  className={`heatmap-cell seq-${level}`}
                  title={label}
                  aria-label={label}
                />
              )
            })}
          </div>
        ))}
      </div>
    </figure>
  )
}
