import { cn } from '@/lib/utils'
import { Card, CardContent } from '@/components/ui/card'
import { JsonViewer } from '@/components/common/JsonViewer'
import type { BillItem } from '@/types'
import { money, rawNumber, rawString, shopeeOrderID } from '@/lib/shopeeBill'
import { FLOW_META } from '../utils/formatters'

interface Props {
  data: Record<string, unknown> | null | undefined
  items?: BillItem[]
}

function FieldRow({
  label,
  value,
  mono = false,
}: {
  label: string
  value: string
  mono?: boolean
}) {
  if (!value) return null
  return (
    <div className="grid gap-1 border-b border-border/50 py-1.5 text-sm last:border-0 sm:grid-cols-[130px_minmax(0,1fr)] sm:gap-3">
      <div className="text-xs font-medium text-muted-foreground">{label}</div>
      <div
        className={cn(
          'flex-1 break-words text-[13px] leading-5 text-foreground',
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
  const orderID = shopeeOrderID(data)
  const orderDateTime = rawString(data, 'order_datetime')
  const sellerName = rawString(data, 'seller_name')
  const buyerName = rawString(data, 'buyer_username')
  const paymentTime = rawString(data, 'payment_time')
  const paymentChannel = rawString(data, 'payment_channel')
  const trackingNo = rawString(data, 'tracking_no')
  const itemCount = rawNumber(data, 'item_count')
  const goodsTotal = rawNumber(data, 'goods_total_amount')
  const shippingAmount = rawNumber(data, 'shipping_amount')
  const paidTotal = rawNumber(data, 'paid_total_amount')
  const note = get('note')
  const file = get('email_file')
  const msgID = get('email_message_id')
  const status = get('status')
  const bodyHTML = get('body_html')
  const bodyText = get('body_text')
  const hasTechnicalData = bodyHTML || bodyText || Object.keys(data).length > 0 || (items?.length ?? 0) > 0
  const isMultiOrderEmailFlow = flow === 'shopee_shipped'
  const sourceTitle =
    flow === 'shopee_excel'
      ? 'ข้อมูลจาก Shopee Excel'
      : flow === 'lazada_excel'
        ? 'ข้อมูลจาก Lazada Excel'
        : flow === 'tiktok_excel'
          ? 'ข้อมูลจาก TikTok Excel/CSV'
          : 'ข้อมูลอีเมลต้นทาง'

  return (
    <Card className="rounded-xl border-border/70 shadow-sm">
      <CardContent className="px-5 py-3">
        <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
          <div>
            <div className="text-sm font-semibold text-foreground">{sourceTitle}</div>
          </div>
          {flowMeta && (
            <div
              className={cn(
                'inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-semibold',
                flowMeta.variant,
              )}
            >
              <span>{flowMeta.label}</span>
            </div>
          )}
        </div>

        <div>
          <FieldRow label="หัวข้ออีเมล" value={subject} />
          <FieldRow label="ผู้ส่ง" value={from} mono />
          {!isMultiOrderEmailFlow && <FieldRow label="วันที่เอกสาร" value={docDate} />}
          {!isMultiOrderEmailFlow && <FieldRow label="หมายเลขคำสั่งซื้อ" value={orderID} mono />}
          {!isMultiOrderEmailFlow && <FieldRow label="วันที่สั่งซื้อ" value={orderDateTime} />}
          {!isMultiOrderEmailFlow && <FieldRow label="ผู้ขาย" value={sellerName} />}
          {!isMultiOrderEmailFlow && <FieldRow label="ผู้ซื้อ" value={buyerName} />}
          {!isMultiOrderEmailFlow && <FieldRow label="เวลาชำระเงิน" value={paymentTime} />}
          {!isMultiOrderEmailFlow && <FieldRow label="ช่องทางชำระเงิน" value={paymentChannel} />}
          {!isMultiOrderEmailFlow && <FieldRow label="เลขพัสดุ" value={trackingNo} mono />}
          {!isMultiOrderEmailFlow && itemCount != null && <FieldRow label="จำนวนสินค้า" value={`${itemCount} รายการ`} />}
          {!isMultiOrderEmailFlow && goodsTotal != null && <FieldRow label="ยอดรวมค่าสินค้า" value={money(goodsTotal)} />}
          {!isMultiOrderEmailFlow && paidTotal != null && <FieldRow label="ยอดที่ชำระ" value={money(paidTotal)} />}
          {isMultiOrderEmailFlow && shippingAmount != null && (
            <FieldRow label="ค่าส่ง / ยอดชำระ" value={`${money(shippingAmount)} / ${money(paidTotal)}`} />
          )}
          <FieldRow label="ลูกค้า" value={customer} />
          <FieldRow label="เบอร์โทร" value={phone} />
          <FieldRow label="หมายเหตุ" value={note} />
          <FieldRow label="ไฟล์แนบ" value={file} mono />
          <FieldRow label="สถานะต้นทาง" value={status} />
          <FieldRow label="Message ID" value={msgID} mono />
        </div>

        {hasTechnicalData && (
          <div className="mt-3 border-t border-border pt-3">
            {(bodyHTML || bodyText) && (
              <details className="rounded-lg border border-border bg-muted/20">
                <summary className="cursor-pointer select-none px-3 py-2 text-xs font-medium text-muted-foreground hover:text-foreground">
                  เปิดดูเนื้อหาอีเมลต้นฉบับ / JSON
                </summary>
                <div className="overflow-hidden border-t border-border bg-background">
                  {bodyHTML ? (
                    <iframe
                      sandbox=""
                      srcDoc={bodyHTML}
                      className="h-[420px] w-full"
                      title="email body"
                    />
                  ) : (
                    <pre className="max-h-[360px] overflow-auto p-3 text-xs whitespace-pre-wrap">
                      {bodyText}
                    </pre>
                  )}
                </div>
              </details>
            )}
            <div className="mt-2">
              <JsonViewer
                title="ข้อมูลทางเทคนิคสำหรับทีม support"
                data={{ raw_data: data, items: items ?? [] }}
                defaultOpen={false}
              />
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  )
}
