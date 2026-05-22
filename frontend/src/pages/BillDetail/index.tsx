import { useEffect, useMemo, useState } from 'react'
import { useLocation, useParams } from 'react-router-dom'
import { useNavigate } from 'react-router-dom'
import { ArrowLeft } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { DetailPageSkeleton } from '@/components/common/LoadingSkeleton'
import type { BillItem } from '@/types'

import { useBillData } from './hooks/useBillData'
import { BillHeader } from './components/BillHeader'
import { BillFailureCard } from './components/BillFailureCard'
import { BillTotal } from './components/BillTotal'
import { BillItemsTable } from './components/BillItemsTable'
import { BillTimeline } from './components/BillTimeline'
import { RawDataCard } from './components/RawDataCard'
import { ArtifactList } from './components/ArtifactList'
import { SmlPayloadSection } from './components/SmlPayloadSection'
import { SendPurchaseDialog } from './components/SendPurchaseDialog'
import { validateForSML } from './utils/validation'
import type { RetryBillPayload } from '@/hooks/useBills'
import { isShopeeSalesBill } from '@/lib/shopeeBill'

export default function BillDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const location = useLocation()
  const { bill, loading, retrying, retryError, reloadBill, handleRetry, handleRetryWithOverride, setBill } =
    useBillData(id)

  // ⚠ All hooks must be declared BEFORE any early return. React tracks hooks
  // by call order; conditional early returns make the count vary between
  // renders and trigger error #310 ("Rendered more hooks than previous").
  // useState + useMemo BOTH live up here. Don't move them below the
  // `if (loading)` guard.

  // highlightItemId — the BillTotal warning card's "ดู →" link sets this so
  // the matching BillItemRow scrolls into view + flashes (1.5s). To re-fire
  // on second click of the same row we briefly null the state in handleJump.
  const [highlightItemId, setHighlightItemId] = useState<string | null>(null)

  // sendDialogOpen — SML 248 documents show a dialog (party picker + WH/VAT)
  // before the retry call, so admin can override per-bill send values.
  const [sendDialogOpen, setSendDialogOpen] = useState(false)

  // Frontend-side validation against backend retry rules. Memo on `bill`
  // so BillTotal/BillItemRow don't recompute on unrelated parent renders.
  // Tolerates bill=null during loading (validateForSML returns no_items).
  const validation = useMemo(
    () => (bill ? validateForSML(bill) : { canSend: false, issues: [], firstBlockingItemId: null }),
    [bill],
  )

  useEffect(() => {
    if (!bill || !id) return
    const route = bill.document_route || bill.preview?.route
    const expectedPath =
      bill.bill_type !== 'sale'
        ? `/bills/${id}`
        : route === 'saleinvoice'
          ? `/sale-invoices/${id}`
          : `/sales-orders/${id}`
    if (location.pathname !== expectedPath) {
      navigate(expectedPath, { replace: true })
    }
  }, [bill, id, location.pathname, navigate])

  const handleJumpToItem = (id: string | null) => {
    if (!id) return
    setHighlightItemId(null)
    // Defer to next tick so the row's useEffect sees null → id transition
    // even if the previous highlight was the same id.
    setTimeout(() => setHighlightItemId(id), 0)
  }

  // Marketplace purchase/sale documents need explicit per-bill SML values.
  const handleSendClick = () => {
    if (bill?.bill_type === 'purchase' || (bill?.bill_type === 'sale' && (bill?.source === 'shopee' || bill?.source === 'lazada' || bill?.source === 'tiktok'))) {
      setSendDialogOpen(true)
    } else {
      handleRetry()
    }
  }

  const handlePurchaseConfirm = async (body: RetryBillPayload) => {
    setSendDialogOpen(false)
    await handleRetryWithOverride(body)
  }

  if (loading) {
    return <DetailPageSkeleton />
  }

  if (!bill) {
    return (
      <div className="space-y-4">
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="gap-1.5 -ml-2 text-muted-foreground"
          onClick={() => navigate(-1)}
        >
          <ArrowLeft className="h-4 w-4" />
          กลับ
        </Button>
        <div className="rounded-md border border-destructive/40 bg-destructive/10 px-4 py-3 text-sm text-destructive">
          ไม่พบบิลที่ต้องการ
        </div>
      </div>
    )
  }

  const total = (bill.items ?? []).reduce(
    (s, i) => s + (i.qty ?? 0) * (i.price ?? 0),
    0,
  )
  const canSend =
    bill.status === 'failed' ||
    bill.status === 'pending' ||
    bill.status === 'needs_review'
  const canEdit = canSend
  const isShopeeSale = isShopeeSalesBill(bill)
  const evidenceSourceLabel = isShopeeSale ? 'ไฟล์ Marketplace Excel' : 'อีเมล'

  const handleItemUpdated = (updated: BillItem) => {
    setBill((prev) => {
      if (!prev) return prev
      return {
        ...prev,
        items: (prev.items ?? []).map((it) =>
          it.id === updated.id ? { ...it, ...updated } : it,
        ),
      }
    })
  }

  const handleItemDeleted = (itemId: string) => {
    setBill((prev) => {
      if (!prev) return prev
      return { ...prev, items: (prev.items ?? []).filter((it) => it.id !== itemId) }
    })
  }

  const handleItemAdded = (newItem: BillItem) => {
    setBill((prev) => {
      if (!prev) return prev
      return { ...prev, items: [...(prev.items ?? []), newItem] }
    })
  }

  return (
    <div className="space-y-4">
      <BillHeader bill={bill} />

      {(bill.error_msg || retryError) && (
        <BillFailureCard errorMsg={bill.error_msg} retryError={retryError} />
      )}

      <BillTotal
        bill={bill}
        total={total}
        retrying={retrying}
        onRetry={handleSendClick}
        validation={validation}
        onJumpToItem={handleJumpToItem}
        expectedRoute={bill.preview?.route}
        expectedEndpoint={bill.preview?.endpoint}
        expectedDocFormat={bill.preview?.doc_format}
      />

      <BillItemsTable
        bill={bill}
        canEdit={canEdit}
        onItemUpdated={handleItemUpdated}
        onItemDeleted={handleItemDeleted}
        onItemAdded={handleItemAdded}
        onRefresh={reloadBill}
        highlightItemId={highlightItemId}
      />

      <section className="space-y-3">
        <div className="flex flex-wrap items-end justify-between gap-3 border-b border-border/70 pb-2">
          <div>
            <h3 className="text-sm font-semibold text-foreground">ข้อมูลประกอบการตรวจสอบ</h3>
            <p className="mt-0.5 text-xs text-muted-foreground">
              ใช้เมื่อต้องย้อนดูที่มาจาก{evidenceSourceLabel} หลักฐานต้นฉบับ และประวัติของบิลนี้
            </p>
          </div>
          <span className="rounded-md bg-muted px-2 py-1 text-xs text-muted-foreground">
            ข้อมูลส่วนนี้ไม่ต้องแก้ก่อนส่ง SML
          </span>
        </div>

        <div className="grid gap-4 xl:grid-cols-[minmax(0,1.18fr)_minmax(380px,0.82fr)]">
          {bill.raw_data && (
            <div className="min-w-0">
              <RawDataCard
                data={bill.raw_data as Record<string, unknown>}
                items={bill.items}
              />
            </div>
          )}

          <div className="min-w-0 space-y-4">
            <ArtifactList billId={bill.id} emailGroup={bill.email_group} />
            <BillTimeline billId={bill.id} shopeeEvents={bill.shopee_events ?? []} />
            <SmlPayloadSection
              smlPayload={bill.sml_payload}
              smlResponse={bill.sml_response}
            />
          </div>
        </div>
      </section>

      {(bill.bill_type === 'purchase' || (bill.bill_type === 'sale' && (bill.source === 'shopee' || bill.source === 'lazada' || bill.source === 'tiktok'))) && (
        <SendPurchaseDialog
          open={sendDialogOpen}
          bill={bill}
          onConfirm={handlePurchaseConfirm}
          onCancel={() => setSendDialogOpen(false)}
        />
      )}
    </div>
  )
}
