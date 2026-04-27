import { StatusDot } from '@/components/common/StatusDot'
import type { BillStatus } from '@/types'

const CONFIG: Record<
  string,
  { label: string; variant: 'success' | 'warning' | 'danger' | 'muted' | 'info' | 'primary' }
> = {
  pending:      { label: 'รอดำเนินการ', variant: 'info' },
  needs_review: { label: 'รอตรวจสอบ',   variant: 'warning' },
  sent:         { label: 'SML สำเร็จ',  variant: 'success' },
  failed:       { label: 'ล้มเหลว',     variant: 'danger' },
  skipped:      { label: 'ข้ามแล้ว',    variant: 'muted' },
}

export default function BillStatusBadge({ status }: { status: BillStatus | string }) {
  const cfg = CONFIG[status] ?? { label: status, variant: 'muted' as const }
  return <StatusDot variant={cfg.variant} label={cfg.label} />
}
