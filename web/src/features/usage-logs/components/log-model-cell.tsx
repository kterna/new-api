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
import { CornerDownRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import type { UsageLog } from '../data/schema'
import { formatModelName, parseLogOther } from '../lib/format'
import { ModelBadge } from './model-badge'

interface LogModelCellProps {
  log: UsageLog
  isAdmin: boolean
}

export function LogModelCell({ log, isAdmin }: LogModelCellProps) {
  const { t } = useTranslation()
  const modelInfo = formatModelName(log)
  const adminInfo = isAdmin ? parseLogOther(log.other)?.admin_info : undefined
  const observedModel =
    adminInfo?.upstream_reported_model ||
    adminInfo?.cpa_model_identity?.upstream_reported_model

  return (
    <div className='flex w-fit max-w-full flex-col gap-0.5'>
      <ModelBadge
        modelName={modelInfo.name}
        actualModel={modelInfo.actualModel}
      />
      {observedModel && observedModel !== log.model_name && (
        <div
          role='note'
          className='text-muted-foreground flex max-w-full items-center gap-1 pl-1 text-xs'
        >
          <CornerDownRight className='size-3 shrink-0' aria-hidden='true' />
          <span className='shrink-0'>{t('Actual Requested Model')}</span>
          <span className='min-w-0 truncate font-mono' title={observedModel}>
            {observedModel}
          </span>
        </div>
      )}
    </div>
  )
}
