import { ReactQueryDevtools } from '@tanstack/react-query-devtools'
import { TanStackRouterDevtools } from '@tanstack/react-router-devtools'
import type { makeRouter } from './router'

export default function Devtools({ router }: { router: ReturnType<typeof makeRouter> }) {
  return (
    <>
      <TanStackRouterDevtools router={router} position="bottom-right" />
      <ReactQueryDevtools buttonPosition="bottom-left" />
    </>
  )
}
