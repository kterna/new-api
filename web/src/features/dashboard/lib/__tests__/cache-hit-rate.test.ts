/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { calculateDashboardStats } from '../stats'

describe('dashboard cache hit rate', () => {
  test('weights cache hits by input tokens across models', () => {
    const stats = calculateDashboardStats([
      { created_at: 1, prompt_tokens: 100, cached_tokens: 50 },
      { created_at: 2, prompt_tokens: 900, cached_tokens: 0 },
    ])

    assert.equal(stats.cacheHitRate, 0.05)
  })

  test('shows zero when older quota rows have no cache token totals', () => {
    const stats = calculateDashboardStats([{ created_at: 1, quota: 10 }])

    assert.equal(stats.cacheHitRate, 0)
  })
})
