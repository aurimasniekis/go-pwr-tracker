import { expect, test } from '@playwright/test'

// The fake upstream serves this playlist, each entry for two consecutive
// polls, then holds the last entry:
//   First Fixture, Second Fixture, (segment), First Fixture, Ąžuolų Fixture (Remix)
// So 10+ observations must collapse into 4 music plays (A → B → A → C).

test('collects, deduplicates and shows airplay', async ({ page, request }) => {
  // Wait until the collector has walked the whole playlist.
  await expect
    .poll(
      async () => {
        const res = await request.get('/api/v1/plays?limit=10')
        const body = (await res.json()) as { items: unknown[] }
        return body.items.length
      },
      { timeout: 30_000 },
    )
    .toBe(4)

  await page.goto('/')
  const nowPlaying = page.getByRole('region', { name: 'Now playing' })
  await expect(nowPlaying.getByText('Ąžuolų Fixture (Remix)')).toBeVisible()
  await expect(nowPlaying.getByText('Live')).toBeVisible()

  // Analytics: 4 plays today, First Fixture played twice.
  await page.getByRole('button', { name: 'Today' }).click()
  await expect(
    page.locator('.tile').filter({ hasText: 'Plays' }).locator('.tile-value'),
  ).toHaveText('4')
  const topTracks = page.getByRole('region', { name: 'Most played tracks' })
  const first = topTracks.getByRole('listitem').first()
  await expect(first).toContainText('First Fixture')
  await expect(first.locator('.rank-count')).toHaveText('2')

  // Activity feed: deduplicated, newest first, deep link works.
  await page.goto('/activity')
  const rows = page.locator('.play-list .play-row')
  await expect(rows).toHaveCount(4)
  await expect(rows.nth(0)).toContainText('Ąžuolų Fixture (Remix)')
  await expect(rows.nth(1)).toContainText('First Fixture')
  await expect(rows.nth(2)).toContainText('Second Fixture')
  await expect(rows.nth(3)).toContainText('First Fixture')

  // Track detail via client-side navigation.
  await rows.nth(1).getByRole('link', { name: 'First Fixture' }).click()
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('First Fixture')
  await expect(page.locator('.facts')).toContainText('2')

  // Unknown API routes are JSON 404s, not the SPA.
  const missing = await request.get('/api/v1/does-not-exist')
  expect(missing.status()).toBe(404)
  expect(missing.headers()['content-type']).toContain('application/json')
})
