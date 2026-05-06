import { AlertTriangle, RefreshCw, Send } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'
import type { Bill } from '@/types'
import { issueLabel, type ValidationResult } from '../utils/validation'

interface Props {
  bill: Bill
  total: number
  retrying: boolean
  onRetry: () => void
  // Frontend-side validation against backend retry rules. When canSend=false
  // the Send button is disabled + a warning card lists the offending issues.
  // Each issue can be clicked to scroll/highlight the first row that hit it.
  validation: ValidationResult
  onJumpToItem: (itemId: string | null) => void
  // expectedRoute / expectedDocFormat — preview of what'll happen when admin
  // clicks Send. Surfaces the SML route + doc_no pattern BEFORE the round-trip
  // so admins can spot misconfigured channels (e.g. shopee bill routed to
  // sale_reserve because endpoint string doesn't match the keywords).
  expectedRoute?: string
  expectedEndpoint?: string
  expectedDocFormat?: string
}

const ROUTE_LABEL: Record<string, string> = {
  sale_reserve: 'SML 213 · ใบสั่งจอง (sale_reserve)',
  saleorder: 'SML 248 · ใบสั่งขาย (saleorder)',
  saleinvoice: 'SML 248 · ใบกำกับภาษี (saleinvoice)',
  purchaseorder: 'SML 248 · ใบสั่งซื้อ (purchaseorder)',
}

export function BillTotal({
  bill,
  total,
  retrying,
  onRetry,
  validation,
  onJumpToItem,
  expectedRoute,
  expectedEndpoint,
  expectedDocFormat,
}: Props) {
  const canShowSendButton =
    bill.status === 'failed' ||
    bill.status === 'pending' ||
    bill.status === 'needs_review'
  const isPurchase = bill.bill_type === 'purchase'
  const isFailed = bill.status === 'failed'

  // Send is enabled only when validation passes AND we're not mid-retry.
  // The disabled state is communicated by both the button's :disabled state
  // and the warning card above (which is the "why" — the button alone
  // wouldn't tell the admin what to fix).
  const enabled = validation.canSend && !retrying

  const buttonLabel = retrying
    ? 'กำลังส่ง...'
    : isFailed
      ? `⚠️ ลองส่งใหม่${isPurchase ? ' (ใบสั่งซื้อ/สั่งจอง)' : ''}`
      : `ยืนยันและส่งไปยัง SML${isPurchase ? ' (ใบสั่งซื้อ/สั่งจอง)' : ''}`

  return (
    <Card>
      <CardContent className="space-y-3 py-4">
        {/* Top row — total + send button */}
        <div className="flex items-center justify-between gap-4">
          <div>
            <div className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
              ยอดรวมทั้งหมด
            </div>
            <div className="mt-0.5 text-2xl font-bold tabular-nums tracking-tight">
              ฿{total.toLocaleString()}
            </div>
          </div>

          {canShowSendButton && (
            <div className="flex flex-col items-end gap-1.5">
              <TooltipProvider delayDuration={150}>
                <Tooltip>
                  <TooltipTrigger asChild>
                    {/* Wrap button in a span so a disabled button still
                        receives hover events (raw <button disabled> swallows
                        them, which means the tooltip wouldn't fire on the
                        very state we most need to explain). */}
                    <span className={!enabled ? 'cursor-not-allowed' : ''}>
                      <Button
                        type="button"
                        onClick={onRetry}
                        disabled={!enabled}
                        variant={isFailed ? 'destructive' : 'default'}
                        className="gap-2 shrink-0"
                      >
                        {retrying ? (
                          <RefreshCw className="h-4 w-4 animate-spin" />
                        ) : isFailed ? (
                          <RefreshCw className="h-4 w-4" />
                        ) : (
                          <Send className="h-4 w-4" />
                        )}
                        {buttonLabel}
                      </Button>
                    </span>
                  </TooltipTrigger>
                  {/* Tooltip only renders content when the button is disabled
                      because of validation — when retrying, the button text
                      already explains itself ("กำลังส่ง..."). */}
                  {!validation.canSend && (
                    <TooltipContent side="left" className="max-w-xs">
                      ยังส่ง SML ไม่ได้ — พบ {validation.issues.length} ปัญหา · ตรวจ item_code / unit_code / qty / price
                    </TooltipContent>
                  )}
                </Tooltip>
              </TooltipProvider>

              {/* Route preview — always visible when send area is shown so
                  admin can see the routing even before validation passes.
                  Dimmed when button is disabled to signal "preview only". */}
              {canShowSendButton && expectedRoute && (
                <div className={cn("text-right text-[10px] tabular-nums text-muted-foreground", !enabled && "opacity-50")}>
                  ↳{' '}
                  <span className="font-medium text-foreground">
                    {ROUTE_LABEL[expectedRoute] ?? expectedRoute}
                  </span>
                  {expectedDocFormat && (
                    <>
                      {' '}· doc_no{' '}
                      <code className="rounded bg-muted px-1 py-0.5 font-mono">
                        {expectedDocFormat}
                      </code>
                    </>
                  )}
                  {expectedEndpoint && expectedEndpoint.startsWith('http') && (
                    <div
                      className="mt-0.5 truncate font-mono text-[10px] text-muted-foreground/70"
                      title={expectedEndpoint}
                    >
                      {expectedEndpoint}
                    </div>
                  )}
                </div>
              )}
            </div>
          )}
        </div>

        {/* Validation warning card — only renders when there are issues to
            fix. Each issue links to the first offending row. Sits between
            the total + button summary and the items table so admin sees
            "what to do" before they look down at items. */}
        {canShowSendButton && !validation.canSend && (
          <div
            className={cn(
              'rounded-md border border-warning/40 bg-warning/[0.06] px-3 py-2.5',
            )}
          >
            <div className="flex items-start gap-2">
              <AlertTriangle
                className="mt-0.5 h-4 w-4 shrink-0 text-warning"
                strokeWidth={2.25}
              />
              <div className="min-w-0 flex-1 space-y-1.5">
                <div className="text-sm font-semibold text-foreground">
                  ยังส่ง SML ไม่ได้ — พบ {validation.issues.length}{' '}
                  ปัญหาที่ต้องแก้
                </div>
                <ul className="space-y-1 text-[13px]">
                  {validation.issues.map((issue) => (
                    <li
                      key={issue.kind}
                      className="flex items-baseline gap-1.5"
                    >
                      <span className="text-muted-foreground/60">•</span>
                      <span className="flex-1 text-foreground">
                        <span className="font-medium tabular-nums">
                          {issue.count}
                        </span>{' '}
                        {issue.kind === 'no_items'
                          ? issueLabel(issue.kind)
                          : `รายการ${issueLabel(issue.kind)}`}
                      </span>
                      {issue.firstItemId && (
                        <button
                          type="button"
                          onClick={() => onJumpToItem(issue.firstItemId)}
                          className="shrink-0 text-[11px] font-medium text-primary hover:underline"
                        >
                          ดู →
                        </button>
                      )}
                    </li>
                  ))}
                </ul>
              </div>
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  )
}
