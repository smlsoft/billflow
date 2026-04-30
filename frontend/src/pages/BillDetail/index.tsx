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
import { validateForSML } from './utils/validation'

export default function BillDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { bill, loading, retrying, retryError, handleRetry, setBill } =
    useBillData(id)

  // highlightItemId is set when admin clicks "ดู →" in the BillTotal warning
  // card. The matching BillItemRow scrolls into view + flashes (1.5s).
  // To re-trigger the effect on second click of the same row, we briefly
  // null the state before setting it (useState batches but we use timer).
  const [highlightItemId, setHighlightItemId] = useState<string | null>(null)
  const handleJumpToItem = (id: string | null) => {
    if (!id) return
    setHighlightItemId(null)
    // Defer to next tick so the row's useEffect sees null → id transition
    // even if the previous highlight was the same id.
    setTimeout(() => setHighlightItemId(id), 0)
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

  // Frontend-side validation against backend retry rules. Memoize so the
  // BillTotal + BillItemRow components don't recompute the issues array on
  // every parent render — items rarely change but parent re-renders often.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  const validation = useMemo(() => validateForSML(bill), [bill])

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
        onRetry={handleRetry}
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
