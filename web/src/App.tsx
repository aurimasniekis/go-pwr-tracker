import { lazy, Suspense, useState } from 'react'
import { type QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider, type RouterHistory } from '@tanstack/react-router'
import { makeRouter } from './router'

// Devtools load only in development builds; with import.meta.env.DEV false
// the dynamic import is dropped from the production bundle.
const Devtools =
  import.meta.env.DEV && import.meta.env.MODE !== 'test' ? lazy(() => import('./devtools')) : null

export function App({
  queryClient,
  history,
}: {
  queryClient: QueryClient
  history?: RouterHistory
}) {
  const [router] = useState(() => makeRouter(queryClient, history))
  return (
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
      {Devtools && (
        <Suspense>
          <Devtools router={router} />
        </Suspense>
      )}
    </QueryClientProvider>
  )
}
