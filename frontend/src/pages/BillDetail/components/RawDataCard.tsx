import { cn } from '@/lib/utils'
import { Card, CardContent } from '@/components/ui/card'
import { JsonViewer } from '@/components/common/JsonViewer'
import type { BillItem } from '@/types'
import { FLOW_META, scoreColor } from '../utils/formatters'

interface Props {
  data: Record<string, unknown> | null | undefined
  items?: BillItem[]
}

function FieldRow({
  icon,
  label,
  value,
  mono = false,
}: {
  icon: string
  label: string
  value: string
  mono?: boolean
}) {
  if (!value) return null
  return (
    <div className="flex gap-3 border-b border-border/50 py-2 text-sm last:border-0">
      <div className="w-7 text-base leading-snug">{icon}</div>
      <div className="min-w-[130px] text-xs text-muted-foreground">{label}</div>
      <div
        className={cn(
          'flex-1 break-words text-foreground',
          mono && 'font-mono text-xs',
        )}
      >
        {value}
      </div>
    </div>
  )
}

export function RawDataCard({ data, items }: Props) {
  if (!data) return null

  const flow = (data.flow as string | undefined) ?? ''
  const flowMeta = FLOW_META[flow]

  const get = (k: string): string => {
    const v = data[k]
    if (v == null) return ''
    return String(v)
  }

  const subject = get('subject')
  const from = get('from')
  const customer = get('customer_name')
  const phone = get('customer_phone')
  const docDate = get('doc_date')
  const note = get('note')
  const file = get('email_file')
  const msgID = get('email_message_id')
  const orderID = get('shopee_order_id') || get('order_id')
  const status = get('status')

  return (
    <Card>
      <CardContent className="pt-4">
        {flowMeta && (
          <div
            className={cn(
              'mb-4 inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-semibold',
              flowMeta.variant,
            )}
          >
            <span>{flowMeta.icon}</span>
            <span>{flowMeta.label}</span>
          </div>
        )}

        <div>
          <FieldRow icon="📧" label="หัวข้ออีเมล" value={subject} />
          <FieldRow icon="📨" label="ผู้ส่ง" value={from} mono />
          <FieldRow icon="🛒" label="หมายเลขคำสั่งซื้อ" value={orderID} mono />
          <FieldRow icon="📅" label="วันที่เอกสาร" value={docDate} />
          <FieldRow icon="👤" label="ลูกค้า" value={customer} />
          <FieldRow icon="📞" label="เบอร์โทร" value={phone} />
          <FieldRow icon="📝" label="หมายเหตุ" value={note} />
          <FieldRow icon="📎" label="ไฟล์แนบ" value={file} mono />
          <FieldRow icon="🏷️" label="สถานะ Shopee" value={status} />
          <FieldRow icon="🆔" label="Message ID" value={msgID} mono />
        </div>

        {/* Items recap */}
        {items && items.length > 0 && (
          <div className="mt-4 border-t border-border pt-4">
            <div className="mb-2 text-xs text-muted-foreground">
              📋 รายการที่ระบบดึงมา ({items.length} รายการ):
            </div>
            <div className="space-y-1">
              {items.map((it) => {
                const candidate = (it.candidates ?? []).find(
                  (c) => c.item_code === it.item_code,
                )
                const score = candidate?.score ?? null
                const scorePct = score != null ? Math.round(score * 100) : null
                const color = scoreColor(score)
                return (
                  <div
                    key={it.id}
                    className="flex items-start gap-2 border-b border-dashed border-border/50 py-1.5 text-sm last:border-0"
                  >
                    <div className="flex-1 break-words text-foreground">
                      {it.raw_name}
                      <div className="mt-0.5 text-xs text-muted-foreground">
                        →{' '}
                        <code
                          style={{ color: it.item_code ? undefined : '#ef4444' }}
                          className="font-mono"
                        >
                          {it.item_code ?? '(ยังไม่ map)'}
                        </code>
                        {candidate?.item_name && (
                          <span className="text-muted-foreground">
                            {' '}· {candidate.item_name}
                          </span>
                        )}
                      </div>
                    </div>
                    <div className="w-24 shrink-0 text-right text-xs text-muted-foreground tabular-nums whitespace-nowrap">
                      {it.qty} × ฿{(it.price ?? 0).toLocaleString()}
                    </div>
                    {scorePct != null && (
                      <div
                        className="w-12 shrink-0 text-center text-xs font-semibold"
                        style={{ color }}
                      >
                        {scorePct}%
                      </div>
                    )}
                  </div>
                )
              })}
            </div>
          </div>
        )}

        {/* Raw JSON dump */}
        <div className="mt-4">
          <JsonViewer
            title="ดู JSON ดิบ (raw_data + items + candidates)"
            data={{ raw_data: data, items: items ?? [] }}
            defaultOpen={false}
          />
        </div>
      </CardContent>
    </Card>
  )
}
