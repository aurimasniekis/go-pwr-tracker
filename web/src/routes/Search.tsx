import { useQuery } from '@tanstack/react-query'
import { getRouteApi, Link } from '@tanstack/react-router'
import { useState, type FormEvent } from 'react'
import { searchQuery } from '../api/queries'
import { Empty, ErrorMessage, Loading } from '../components/QueryStates'

const route = getRouteApi('/search')

export function Search() {
  const { q } = route.useSearch()
  const navigate = route.useNavigate()
  const [text, setText] = useState(q ?? '')
  const results = useQuery({ ...searchQuery(q ?? ''), enabled: !!q })

  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    navigate({ search: { q: text.trim() || undefined } })
  }

  return (
    <section className="card">
      <h1>Search</h1>
      <form className="filters" role="search" onSubmit={onSubmit}>
        <label className="grow">
          <span>Track title or artist name</span>
          <input
            type="search"
            value={text}
            onChange={(e) => setText(e.target.value)}
            maxLength={100}
            autoFocus
          />
        </label>
        <button type="submit" className="btn">
          Search
        </button>
      </form>
      {!q ? (
        <Empty>Type a title or artist to search the airplay history.</Empty>
      ) : results.isPending ? (
        <Loading />
      ) : results.isError ? (
        <ErrorMessage error={results.error} onRetry={() => results.refetch()} />
      ) : results.data.tracks.length === 0 && results.data.artists.length === 0 ? (
        <Empty>No tracks or artists match “{q}”.</Empty>
      ) : (
        <div className="grid-2">
          <div>
            <h2>Artists</h2>
            {results.data.artists.length === 0 ? (
              <p className="muted">No artists.</p>
            ) : (
              <ul className="result-list">
                {results.data.artists.map((a) => (
                  <li key={a.id}>
                    <Link to="/artists/$artistId" params={{ artistId: a.id }}>
                      {a.name}
                    </Link>
                    <span className="muted"> · {a.total_plays} plays</span>
                  </li>
                ))}
              </ul>
            )}
          </div>
          <div>
            <h2>Tracks</h2>
            {results.data.tracks.length === 0 ? (
              <p className="muted">No tracks.</p>
            ) : (
              <ul className="result-list">
                {results.data.tracks.map((t) => (
                  <li key={t.id}>
                    <Link to="/tracks/$trackId" params={{ trackId: t.id }}>
                      {t.title}
                    </Link>
                    {t.artist && <span className="muted"> · {t.artist.name}</span>}
                    <span className="muted"> · {t.total_plays} plays</span>
                  </li>
                ))}
              </ul>
            )}
          </div>
        </div>
      )}
    </section>
  )
}
