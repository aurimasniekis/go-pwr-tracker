import { render } from '@testing-library/react'
import { createMemoryHistory } from '@tanstack/react-router'
import { QueryClient } from '@tanstack/react-query'
import { vi } from 'vitest'
import { App } from '../App'

type Handler = (url: URL) => { status?: number; body: unknown } | undefined

/**
 * Renders the full app at `path` with fetch stubbed by `handler`. Unhandled
 * API paths return an empty successful response appropriate to the route.
 */
export function renderApp(path: string, handler: Handler) {
  const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
    const url = new URL(String(input), 'http://localhost')
    const res = handler(url)
    if (!res) throw new Error(`unhandled request ${url.pathname}`)
    return new Response(JSON.stringify(res.body), {
      status: res.status ?? 200,
      headers: { 'Content-Type': 'application/json' },
    })
  })
  vi.stubGlobal('fetch', fetchMock)
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const history = createMemoryHistory({ initialEntries: [path] })
  const utils = render(<App queryClient={queryClient} history={history} />)
  return { ...utils, fetchMock, queryClient }
}
