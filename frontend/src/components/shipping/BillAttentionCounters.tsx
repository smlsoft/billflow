import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import type { BillAttentionCounts } from '@/hooks/useBillAttentionCounts'

export function BillAttentionCounters({
  counts,
  error,
  refresh,
  open,
  selected,
}: {
  counts: BillAttentionCounts | null
  error: boolean
  refresh: () => void
  open: (kind: 'shipping' | 'email') => void
  selected?: 'shipping' | 'email'
}) {
  return (
    <div
      className="flex flex-wrap items-center gap-2"
      aria-label="งานค้างที่ต้องตรวจ"
    >
      {(['shipping', 'email'] as const).map((kind) => {
        const count = counts?.[kind]
        return (
          <Button
            key={kind}
            variant="outline"
            size="sm"
            disabled={counts === null}
            aria-pressed={selected === kind}
            onClick={() => open(kind)}
            title="จำนวนบิลค้างทั้งหมด กดเพื่อเปิดรายการและล้างตัวกรองอื่น"
            className={cn(
              'h-auto min-h-8 max-w-full gap-1.5 py-1 text-xs',
              count && count > 0 ? 'border-warning/50 text-foreground' : '',
              selected === kind && 'bg-accent',
            )}
          >
            <span className="min-w-0 whitespace-normal">
              {kind === 'shipping' ? 'จัดส่งต้องตรวจ' : 'อีเมลต้องตรวจ'}
            </span>
            <span
              className={cn(
                'min-w-5 shrink-0 rounded px-1.5 py-0.5 text-center font-semibold tabular-nums',
                count && count > 0 ? 'bg-warning/15' : 'bg-muted',
              )}
            >
              {count == null
                ? error
                  ? '—'
                  : '…'
                : count.toLocaleString('th-TH')}
            </span>
            <span className="sr-only">บิล</span>
          </Button>
        )
      })}
      {error ? (
        <span role="status" className="text-xs text-muted-foreground">
          โหลดจำนวนไม่สำเร็จ{' '}
          <Button
            variant="link"
            size="sm"
            className="h-auto p-0 text-xs"
            onClick={refresh}
          >
            ลองใหม่
          </Button>
        </span>
      ) : (
        <span className="text-[11px] text-muted-foreground">
          บิลค้างทั้งหมด · อัปเดตทุก 30 วินาที · บิลอาจซ้ำกันระหว่างสองกลุ่ม
        </span>
      )}
    </div>
  )
}
