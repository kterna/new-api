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
import { useCallback, useEffect, useMemo, useState } from 'react'
import { BarChart3, CalendarClock, Loader2, RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import dayjs from '@/lib/dayjs'
import {
  formatLogQuota,
  formatNumber,
  formatTimestampToDate,
  formatTokens,
} from '@/lib/format'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { getChannelUsageStats, type ChannelUsageStats } from '../../api'
import { useChannels } from '../channels-provider'

type ChannelUsageDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

type PresetKey = 'today' | 'sevenDays' | 'thirtyDays'

type UsageRange = {
  key: PresetKey
  label: string
  start: number
  end: number
}

type UsageStatsCardProps = {
  label: string
  stats: ChannelUsageStats | null
  loading?: boolean
}

function dateTimeInputValue(timestamp: number) {
  return dayjs(timestamp * 1000).format('YYYY-MM-DDTHH:mm')
}

function UsageStatsCard({ label, stats, loading }: UsageStatsCardProps) {
  const { t } = useTranslation()

  if (loading) {
    return (
      <div className='border-border/70 rounded-lg border p-4'>
        <Skeleton className='mb-4 h-5 w-24' />
        <div className='grid gap-3'>
          <Skeleton className='h-12' />
          <Skeleton className='h-12' />
          <Skeleton className='h-12' />
        </div>
      </div>
    )
  }

  const quota = stats?.quota || 0
  const requests = stats?.requests || 0
  const tokens = stats?.tokens || 0

  return (
    <div className='border-border/70 rounded-lg border p-4'>
      <div className='mb-3 flex items-center justify-between gap-3'>
        <div className='flex items-center gap-2 text-sm font-medium'>
          <CalendarClock className='text-muted-foreground size-4' />
          <span>{label}</span>
        </div>
        {stats && (
          <span className='text-muted-foreground text-xs'>
            {formatTimestampToDate(stats.start_timestamp)} -{' '}
            {formatTimestampToDate(stats.end_timestamp)}
          </span>
        )}
      </div>

      <div className='grid gap-3'>
        <div>
          <div className='text-muted-foreground text-xs'>{t('Usage')}</div>
          <div className='mt-1 font-mono text-sm font-semibold'>
            {formatLogQuota(quota)}
          </div>
        </div>
        <div>
          <div className='text-muted-foreground text-xs'>{t('Requests')}</div>
          <div className='mt-1 font-mono text-sm font-semibold'>
            {formatNumber(requests)}
          </div>
        </div>
        <div>
          <div className='text-muted-foreground text-xs'>{t('Tokens')}</div>
          <div className='mt-1 font-mono text-sm font-semibold'>
            {formatTokens(tokens)}
          </div>
        </div>
      </div>
    </div>
  )
}

export function ChannelUsageDialog({
  open,
  onOpenChange,
}: ChannelUsageDialogProps) {
  const { t } = useTranslation()
  const { currentRow } = useChannels()
  const [loading, setLoading] = useState(false)
  const [customLoading, setCustomLoading] = useState(false)
  const [presetStats, setPresetStats] = useState<
    Partial<Record<PresetKey, ChannelUsageStats>>
  >({})
  const [customStats, setCustomStats] = useState<ChannelUsageStats | null>(null)

  const now = useMemo(() => dayjs(), [])
  const [customStart, setCustomStart] = useState(() =>
    dateTimeInputValue(now.startOf('day').unix())
  )
  const [customEnd, setCustomEnd] = useState(() =>
    dateTimeInputValue(now.unix())
  )

  const ranges = useMemo<UsageRange[]>(() => {
    const current = dayjs()
    return [
      {
        key: 'today',
        label: t('Today'),
        start: current.startOf('day').unix(),
        end: current.unix(),
      },
      {
        key: 'sevenDays',
        label: t('Last 7 days'),
        start: current.subtract(6, 'day').startOf('day').unix(),
        end: current.unix(),
      },
      {
        key: 'thirtyDays',
        label: t('Last 30 days'),
        start: current.subtract(29, 'day').startOf('day').unix(),
        end: current.unix(),
      },
    ]
  }, [t])

  const loadPresetStats = useCallback(async () => {
    const row = currentRow
    if (!row) return

    setLoading(true)
    try {
      const responses = await Promise.all(
        ranges.map(async (range) => {
          const response = await getChannelUsageStats(row.id, {
            start_timestamp: range.start,
            end_timestamp: range.end,
          })
          if (!response.success || !response.data) {
            throw new Error(response.message || t('Failed to fetch usage'))
          }
          return [range.key, response.data] as const
        })
      )
      setPresetStats(Object.fromEntries(responses))
    } catch (error: unknown) {
      toast.error(
        error instanceof Error ? error.message : t('Failed to fetch usage')
      )
    } finally {
      setLoading(false)
    }
  }, [currentRow, ranges, t])

  useEffect(() => {
    if (!open || !currentRow) return

    const current = dayjs()
    setCustomStart(dateTimeInputValue(current.startOf('day').unix()))
    setCustomEnd(dateTimeInputValue(current.unix()))
    setCustomStats(null)
    loadPresetStats()
  }, [open, currentRow, loadPresetStats])

  const handleLoadCustom = async () => {
    const row = currentRow
    if (!row) return

    const start = dayjs(customStart)
    const end = dayjs(customEnd)
    if (!start.isValid() || !end.isValid()) {
      toast.error(t('Invalid time range'))
      return
    }
    if (start.unix() > end.unix()) {
      toast.error(t('Start time must be earlier than end time'))
      return
    }

    setCustomLoading(true)
    try {
      const response = await getChannelUsageStats(row.id, {
        start_timestamp: start.unix(),
        end_timestamp: end.unix(),
      })
      if (!response.success || !response.data) {
        throw new Error(response.message || t('Failed to fetch usage'))
      }
      setCustomStats(response.data)
    } catch (error: unknown) {
      toast.error(
        error instanceof Error ? error.message : t('Failed to fetch usage')
      )
    } finally {
      setCustomLoading(false)
    }
  }

  const handleClose = (nextOpen: boolean) => {
    if (!nextOpen) {
      setPresetStats({})
      setCustomStats(null)
    }
    onOpenChange(nextOpen)
  }

  if (!currentRow) return null

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent className='sm:max-w-3xl'>
        <DialogHeader>
          <DialogTitle className='flex items-center gap-2'>
            <BarChart3 className='size-5' />
            {t('Usage Statistics')}
          </DialogTitle>
          <DialogDescription>
            {currentRow.name} #{currentRow.id}
          </DialogDescription>
        </DialogHeader>

        <div className='space-y-4'>
          <div className='grid gap-4 lg:grid-cols-3'>
            {ranges.map((range) => (
              <UsageStatsCard
                key={range.key}
                label={range.label}
                stats={presetStats[range.key] || null}
                loading={loading}
              />
            ))}
          </div>

          <div className='border-border/70 rounded-lg border p-4'>
            <div className='mb-3 flex items-center justify-between gap-3'>
              <div className='text-sm font-medium'>{t('Custom range')}</div>
              <Button
                variant='outline'
                size='sm'
                onClick={handleLoadCustom}
                disabled={customLoading}
              >
                {customLoading ? (
                  <Loader2 className='size-4 animate-spin' />
                ) : (
                  <RefreshCw className='size-4' />
                )}
                {t('Refresh')}
              </Button>
            </div>

            <div className='grid gap-3 sm:grid-cols-2'>
              <Input
                type='datetime-local'
                value={customStart}
                aria-label={t('Start time')}
                onChange={(event) => setCustomStart(event.target.value)}
              />
              <Input
                type='datetime-local'
                value={customEnd}
                aria-label={t('End time')}
                onChange={(event) => setCustomEnd(event.target.value)}
              />
            </div>

            {(customStats || customLoading) && (
              <div className='mt-4'>
                <UsageStatsCard
                  label={t('Custom range')}
                  stats={customStats}
                  loading={customLoading}
                />
              </div>
            )}
          </div>
        </div>

        <DialogFooter>
          <Button
            variant='outline'
            onClick={() => handleClose(false)}
            disabled={loading || customLoading}
          >
            {t('Close')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
