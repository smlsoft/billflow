import { useId, useState, useEffect } from 'react'
import dayjs from 'dayjs'
import { CalendarDays, ChevronLeft, ChevronRight } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { cn } from '@/lib/utils'

interface DateRangePickerProps {
  from: string
  to: string
  onFromChange: (value: string) => void
  onToChange: (value: string) => void
  className?: string
  title?: string
  description?: string
  presets?: DateRangePreset[]
  clearLabel?: string
}

export type DateRangePreset = {
  label: string
  getRange: () => { from: string; to: string }
}

const THAI_MONTHS = [
  'มกราคม', 'กุมภาพันธ์', 'มีนาคม', 'เมษายน', 'พฤษภาคม', 'มิถุนายน',
  'กรกฎาคม', 'สิงหาคม', 'กันยายน', 'ตุลาคม', 'พฤศจิกายน', 'ธันวาคม',
]

const WEEKDAY_LABELS = ['จ', 'อ', 'พ', 'พฤ', 'ศ', 'ส', 'อา']

const defaultPresets: DateRangePreset[] = [
  {
    label: 'วันนี้',
    getRange: () => {
      const today = dayjs().format('YYYY-MM-DD')
      return { from: today, to: today }
    },
  },
  {
    label: '7 วัน',
    getRange: () => ({
      from: dayjs().subtract(6, 'day').format('YYYY-MM-DD'),
      to: dayjs().format('YYYY-MM-DD'),
    }),
  },
  {
    label: 'เดือนนี้',
    getRange: () => ({
      from: dayjs().startOf('month').format('YYYY-MM-DD'),
      to: dayjs().format('YYYY-MM-DD'),
    }),
  },
]

function displayDate(value: string): string {
  return value ? dayjs(value).format('DD/MM/YY') : ''
}

interface MiniCalendarProps {
  month: dayjs.Dayjs
  from: string
  to: string
  hoverDate: string | null
  onSelect: (date: string) => void
  onHover: (date: string | null) => void
  onPrevMonth: () => void
  onNextMonth: () => void
}

function MiniCalendar({ month, from, to, hoverDate, onSelect, onHover, onPrevMonth, onNextMonth }: MiniCalendarProps) {
  const cells: Array<dayjs.Dayjs | null> = []
  const firstDay = month.startOf('month')
  const startOffset = (firstDay.day() + 6) % 7 // Monday-first
  for (let i = 0; i < startOffset; i++) cells.push(null)
  for (let d = 0; d < month.daysInMonth(); d++) cells.push(firstDay.add(d, 'day'))
  while (cells.length < 42) cells.push(null)

  const todayStr = dayjs().format('YYYY-MM-DD')

  // Effective to: use hoverDate as preview when only 'from' is set
  const effectiveTo = from && !to && hoverDate ? hoverDate : to
  const rangeStart = from && effectiveTo ? (from < effectiveTo ? from : effectiveTo) : null
  const rangeEnd = from && effectiveTo ? (from < effectiveTo ? effectiveTo : from) : null

  return (
    <div className="select-none">
      {/* Month header */}
      <div className="flex items-center justify-between mb-1.5">
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="h-6 w-6"
          onClick={onPrevMonth}
        >
          <ChevronLeft className="h-3.5 w-3.5" />
        </Button>
        <span className="text-xs font-medium">
          {THAI_MONTHS[month.month()]} {month.year() + 543}
        </span>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="h-6 w-6"
          onClick={onNextMonth}
        >
          <ChevronRight className="h-3.5 w-3.5" />
        </Button>
      </div>

      {/* Weekday headers */}
      <div className="grid grid-cols-7 mb-0.5">
        {WEEKDAY_LABELS.map((d) => (
          <div
            key={d}
            className="flex h-6 items-center justify-center text-[10px] text-muted-foreground font-medium"
          >
            {d}
          </div>
        ))}
      </div>

      {/* Day grid */}
      <div className="grid grid-cols-7">
        {cells.map((d, i) => {
          if (!d) {
            return <div key={i} className="h-7 w-full" />
          }

          const dateStr = d.format('YYYY-MM-DD')
          const isFrom = dateStr === from
          const isTo = dateStr === to
          const isSelected = isFrom || isTo
          const isDifferentRange = from !== to
          const isInRange =
            rangeStart && rangeEnd && isDifferentRange &&
            dateStr > rangeStart && dateStr < rangeEnd
          const isRangeStart = rangeStart && rangeEnd && isDifferentRange && dateStr === rangeStart
          const isRangeEnd = rangeStart && rangeEnd && isDifferentRange && dateStr === rangeEnd
          const isToday = dateStr === todayStr

          return (
            <div
              key={i}
              className={cn(
                'flex h-7 items-center justify-center',
                isInRange && 'bg-primary/15',
                isRangeStart && 'bg-primary/15 rounded-l-full',
                isRangeEnd && 'bg-primary/15 rounded-r-full',
              )}
            >
              <button
                type="button"
                className={cn(
                  'flex h-7 w-7 items-center justify-center rounded-full text-xs cursor-pointer transition-colors',
                  isSelected && 'bg-primary text-primary-foreground font-semibold',
                  isToday && !isSelected && 'ring-1 ring-primary ring-offset-1',
                  !isSelected && 'hover:bg-accent',
                )}
                onClick={() => onSelect(dateStr)}
                onMouseEnter={() => onHover(dateStr)}
                onMouseLeave={() => onHover(null)}
              >
                {d.date()}
              </button>
            </div>
          )
        })}
      </div>
    </div>
  )
}

export function DateRangePicker({
  from,
  to,
  onFromChange,
  onToChange,
  className,
  title = 'ช่วงวันที่',
  description = 'ใช้กรองประวัติการทำงานตามวันที่เกิดรายการ',
  presets = defaultPresets,
  clearLabel = 'ล้างช่วงวันที่',
}: DateRangePickerProps) {
  const id = useId()
  const label = from || to
    ? `${displayDate(from) || 'เริ่มต้น'} - ${displayDate(to) || 'วันนี้'}`
    : 'เลือกช่วงวันที่'

  const [calendarMonth, setCalendarMonth] = useState<dayjs.Dayjs>(
    () => (from ? dayjs(from) : dayjs()).startOf('month')
  )
  const [selectingStep, setSelectingStep] = useState<'from' | 'to'>('from')
  const [hoverDate, setHoverDate] = useState<string | null>(null)

  useEffect(() => {
    if (from) setCalendarMonth(dayjs(from).startOf('month'))
  }, [from])

  function handleCalendarSelect(dateStr: string) {
    if (selectingStep === 'from') {
      onFromChange(dateStr)
      onToChange('')
      setSelectingStep('to')
    } else {
      if (from && dateStr < from) {
        onToChange(from)
        onFromChange(dateStr)
      } else {
        onToChange(dateStr)
      }
      setSelectingStep('from')
      setHoverDate(null)
    }
  }

  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="outline"
          className={cn('h-10 min-w-[210px] justify-start gap-2 px-3 font-normal', className)}
        >
          <CalendarDays className="h-3.5 w-3.5 text-muted-foreground" />
          <span className={cn('text-sm', !(from || to) && 'text-muted-foreground')}>
            {label}
          </span>
        </Button>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-[min(320px,calc(100vw-2rem))] p-2.5">
        <div className="space-y-2.5">
          <div>
            <div className="text-sm font-medium">{title}</div>
            <div className="mt-0.5 text-xs text-muted-foreground">
              {description}
            </div>
          </div>

          <div className="grid grid-cols-3 gap-1.5">
            {presets.map((preset) => (
              <Button
                key={preset.label}
                type="button"
                variant="secondary"
                size="sm"
                className="h-7 px-2 text-xs"
                onClick={() => {
                  const range = preset.getRange()
                  onFromChange(range.from)
                  onToChange(range.to)
                  setCalendarMonth(dayjs(range.from).startOf('month'))
                  setSelectingStep('from')
                  setHoverDate(null)
                }}
              >
                {preset.label}
              </Button>
            ))}
          </div>

          <MiniCalendar
            month={calendarMonth}
            from={from}
            to={to}
            hoverDate={selectingStep === 'to' ? hoverDate : null}
            onSelect={handleCalendarSelect}
            onHover={setHoverDate}
            onPrevMonth={() => setCalendarMonth((m) => m.subtract(1, 'month'))}
            onNextMonth={() => setCalendarMonth((m) => m.add(1, 'month'))}
          />

          {selectingStep === 'to' && (
            <p className="text-center text-[10px] text-muted-foreground -mt-1">
              เลือกวันสิ้นสุด
            </p>
          )}

          <div className="grid grid-cols-2 gap-2">
            <div className="space-y-1">
              <Label htmlFor={`${id}-date-range-from`} className="text-xs text-muted-foreground">
                ตั้งแต่
              </Label>
              <Input
                id={`${id}-date-range-from`}
                value={from}
                onChange={(e) => onFromChange(e.target.value)}
                placeholder="YYYY-MM-DD"
                className="h-8 font-mono text-xs"
              />
            </div>
            <div className="space-y-1">
              <Label htmlFor={`${id}-date-range-to`} className="text-xs text-muted-foreground">
                ถึง
              </Label>
              <Input
                id={`${id}-date-range-to`}
                value={to}
                onChange={(e) => onToChange(e.target.value)}
                placeholder="YYYY-MM-DD"
                className="h-8 font-mono text-xs"
              />
            </div>
          </div>

          {(from || to) && (
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="h-7 w-full text-xs"
              onClick={() => {
                onFromChange('')
                onToChange('')
                setSelectingStep('from')
                setHoverDate(null)
              }}
            >
              {clearLabel}
            </Button>
          )}
        </div>
      </PopoverContent>
    </Popover>
  )
}
