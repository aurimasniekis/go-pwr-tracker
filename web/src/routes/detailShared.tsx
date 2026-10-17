import { useQuery } from '@tanstack/react-query'
import { airplayQuery, hourlyQuery, summaryQuery } from '../api/queries'
import type { StatsParams } from '../api/client'
import { AirplayChart } from '../components/AirplayChart'
import { HourlyChart, WeekdayHeatmap } from '../components/HourlyChart'
import { ErrorMessage, Loading } from '../components/QueryStates'
import { CoverageNote, SummaryTiles } from '../components/SummaryTiles'
import { bucketFor, type RangePreset } from '../lib/range'

/** Trend, summary and hour-of-day charts for a track or artist. */
export function EntityAnalytics({
  params,
  preset,
  timeZone,
  showDistinctTiles = true,
}: {
  params: StatsParams
  preset: RangePreset
  timeZone: string
  showDistinctTiles?: boolean
}) {
  const summary = useQuery(summaryQuery(params))
  const trend = useQuery(airplayQuery({ ...params, bucket: bucketFor(preset) }))
  const hourly = useQuery(hourlyQuery(params))
  return (
    <>
      {summary.isPending ? (
        <Loading />
      ) : summary.isError ? (
        <ErrorMessage error={summary.error} onRetry={() => summary.refetch()} />
      ) : (
        <div className={showDistinctTiles ? undefined : 'tiles-single'}>
          <SummaryTiles data={summary.data} />
          <CoverageNote data={summary.data} timeZone={timeZone} />
        </div>
      )}
      <section className="card">
        {trend.isPending ? (
          <Loading />
        ) : trend.isError ? (
          <ErrorMessage error={trend.error} onRetry={() => trend.refetch()} />
        ) : (
          <AirplayChart data={trend.data} title="Airplay trend" />
        )}
      </section>
      <section className="card">
        {hourly.isPending ? (
          <Loading />
        ) : hourly.isError ? (
          <ErrorMessage error={hourly.error} onRetry={() => hourly.refetch()} />
        ) : (
          <>
            <HourlyChart data={hourly.data} />
            <WeekdayHeatmap data={hourly.data} />
          </>
        )}
      </section>
    </>
  )
}
