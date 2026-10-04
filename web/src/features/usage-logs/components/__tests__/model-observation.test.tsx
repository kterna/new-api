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

import { Window } from 'happy-dom'
import { createInstance } from 'i18next'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { I18nextProvider, initReactI18next } from 'react-i18next'

import { usageLogSchema } from '../../data/schema'
import { LogModelCell } from '../log-model-cell'

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'zh',
  resources: {
    zh: { translation: { 'Actual Requested Model': '实际请求模型' } },
  },
})

const log = usageLogSchema.parse({
  id: 125186,
  user_id: 1,
  created_at: 0,
  type: 2,
  content: '',
  model_name: 'gpt-6-astra',
  other: JSON.stringify({
    admin_info: {
      cpa_model_identity: { upstream_reported_model: 'gpt-6-luna' },
    },
  }),
})

function renderModelCell(isAdmin: boolean, other = log.other) {
  const window = new Window()
  const html = renderToStaticMarkup(
    createElement(
      I18nextProvider,
      { i18n },
      createElement(LogModelCell, { log: { ...log, other }, isAdmin })
    )
  )
  const container = window.document.createElement('div')
  container.innerHTML = html
  return { window, container }
}

describe('usage log model observation', () => {
  test('shows a different upstream model beneath the requested model to admins', () => {
    const { window, container } = renderModelCell(true)
    const note = container.querySelector('[role="note"]')

    assert.ok(note)
    assert.match(note.textContent ?? '', /实际请求模型.*gpt-6-luna/)
    assert.ok(note.querySelector('svg[aria-hidden="true"]'))
    const content = container.textContent ?? ''
    assert.ok(content.indexOf('gpt-6-astra') < content.indexOf('gpt-6-luna'))
    window.close()
  })

  test('does not expose the upstream model to non-admins', () => {
    const { window, container } = renderModelCell(false)

    assert.equal(container.querySelector('[role="note"]'), null)
    assert.ok(!container.textContent?.includes('gpt-6-luna'))
    window.close()
  })

  test('keeps a single model line when the upstream reports the requested name', () => {
    const matchingOther = JSON.stringify({
      admin_info: {
        cpa_model_identity: { upstream_reported_model: 'gpt-6-astra' },
      },
    })
    const { window, container } = renderModelCell(true, matchingOther)

    assert.equal(container.querySelector('[role="note"]'), null)
    window.close()
  })
})
