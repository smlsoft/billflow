import { useMemo, useState } from 'react'
import { useParams } from 'react-router-dom'
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

export default function BillDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { bill, loading, retrying, retryError, handleRetry, handleRetryWithOverride, setBill } =
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

  // sendDialogOpen — purchase bills show a dialog (supplier picker + remark)
  // before the actual retry call, so admin can override party_code and add a note.
  const [sendDialogOpen, setSendDialogOpen] = useState(false)

  // Frontend-side validation against backend retry rules. Memo on `bill`
  // so BillTotal/BillItemRow don't recompute on unrelated parent renders.
  // Tolerates bill=null during loading (validateForSML returns no_items).
  const validation = useMemo(
    () => (bill ? validateForSML(bill) : { canSend: false, issues: [], firstBlockingItemId: null }),
    [bill],
  )

  const handleJumpToItem = (id: string | null) => {
    if (!id) return
    setHighlightItemId(null)
    // Defer to next tick so the row's useEffect sees null → id transition
    // even if the previous highlight was the same id.
    setTimeout(() => setHighlightItemId(id), 0)
  }

  // For purchase bills, open the supplier+remark dialog instead of retrying directly.
  const handleSendClick = () => {
    if (bill?.bill_type === 'purchase') {
      setSendDialogOpen(true)
    } else {
      handleRetry()
    }
  }

  const handlePurchaseConfirm = async (partyCode: string, remark: string) => {
    setSendDialogOpen(false)
    await handleRetryWithOverride(partyCode, remark)
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
    // space-y-6 (was 4) — bill detail has 8 stacked sections (header, failure
    // card, total, items, raw data, artifacts, sml payload, timeline). The
    // tighter spacing made everything feel cramped on smaller screens.
    <div className="space-y-6">
      <BillHeader bill={bill} />

      {/* Structured failure card — only renders when the bill has a stored
          error or there's a fresh retry error. Replaces the previous
          inline red text under BillHeader; admin can copy + send to dev. */}
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

      {bill.bill_type === 'purchase' && (
        <SendPurchaseDialog
          open={sendDialogOpen}
          onConfirm={handlePurchaseConfirm}
          onCancel={() => setSendDialogOpen(false)}
        />
      )}

      <BillItemsTable
        bill={bill}
        canEdit={canEdit}
        onItemUpdated={handleItemUpdated}
        onItemDeleted={handleItemDeleted}
        onItemAdded={handleItemAdded}
        highlightItemId={highlightItemId}
      />

      {bill.raw_data && (
        <div className="space-y-2">
          <h3 className="text-sm font-semibold text-foreground">ข้อมูลที่รับมา</h3>
          <RawDataCard
            data={bill.raw_data as Record<string, unknown>}
            items={bill.items}
          />
        </div>
      )}

      <ArtifactList billId={bill.id} />

      <SmlPayloadSection
        smlPayload={bill.sml_payload}
        smlResponse={bill.sml_response}
      />

      {/* Activity timeline for this bill — answers "ทำไมบิลนี้ถึงเป็นแบบนี้"
          without leaving the page. Joins audit_logs ON target_id = bill.id. */}
      <BillTimeline billId={bill.id} />
    </div>
  )
}
