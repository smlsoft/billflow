import { useEffect, useMemo, useState } from 'react'
import { useLocation, useParams } from 'react-router-dom'
import { useNavigate } from 'react-router-dom'
import { ArrowLeft, CreditCard, UserCog } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { DetailPageSkeleton } from '@/components/common/LoadingSkeleton'
import type { BillItem } from '@/types'

import { useBillData } from './hooks/useBillData'
import { useAuth } from '@/hooks/useAuth'
import { BillHeader } from './components/BillHeader'
import { BillFailureCard } from './components/BillFailureCard'
import { BillTotal } from './components/BillTotal'
import { BillItemsTable } from './components/BillItemsTable'
import { BillTimeline } from './components/BillTimeline'
import { ArtifactList } from './components/ArtifactList'
import { SmlPayloadSection } from './components/SmlPayloadSection'
import { SendPurchaseDialog } from './components/SendPurchaseDialog'
import { SMLSendProgressDialog, type SMLSendProgressStatus } from './components/SMLSendProgressDialog'
import { UpdatePurchaseCreditorDialog } from './components/UpdatePurchaseCreditorDialog'
import { UpdatePrintPaymentMethodDialog } from './components/UpdatePrintPaymentMethodDialog'
import { validateForSML } from './utils/validation'
import { updateBillPrintPaymentMethod, updateBillPurchaseCreditor, type RetryBillPayload } from '@/hooks/useBills'
import { useSMLReadiness } from '@/hooks/useSMLReadiness'
import { humanizeSMLConnectionError, isSMLReady, smlBlockedMessage } from '@/lib/sml-readiness'
import type { Party } from '@/pages/ChannelDefaults/PartyPicker'

type SingleSMLSendResult = {
  docNo?: string | null
  bill?: {
    sml_doc_no?: string | null
  } | null
}

type SendProgressState = {
  open: boolean
  status: SMLSendProgressStatus
  docNo: string | null
  error: string | null
}

type BillDetailLocationState = {
  returnTo?: unknown
}

function validateBillReturnTo(value: unknown): string | null {
  if (typeof value !== 'string') return null
  const path = value.trim()
  if (!path || !path.startsWith('/') || path.startsWith('//')) return null
  const allowedListPaths = ['/bills', '/sales-orders', '/sale-invoices']
  return allowedListPaths.some((prefix) => path === prefix || path.startsWith(`${prefix}?`))
    ? path
    : null
}

export default function BillDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const location = useLocation()
  const { user } = useAuth()
  const {
    bill,
    loading,
    retrying,
    regeneratingDocNo,
    refreshingDocNo,
    retryError,
    reloadBill,
    handleRetry,
    handleRetryWithOverride,
    handleRegenerateDocNo,
    handleFetchLatestDocNo,
    setBill,
  } =
    useBillData(id)
  const { readiness: smlReadiness, loading: smlReadinessLoading } = useSMLReadiness()

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
  const [creditorDialogOpen, setCreditorDialogOpen] = useState(false)
  const [creditorUpdating, setCreditorUpdating] = useState(false)
  const [paymentDialogOpen, setPaymentDialogOpen] = useState(false)
  const [paymentUpdating, setPaymentUpdating] = useState(false)
  const [sendProgress, setSendProgress] = useState<SendProgressState>({
    open: false,
    status: 'sending',
    docNo: null,
    error: null,
  })

  // Frontend-side validation against backend retry rules. Memo on `bill`
  // so BillTotal/BillItemRow don't recompute on unrelated parent renders.
  // Tolerates bill=null during loading (validateForSML returns no_items).
  const validation = useMemo(
    () => (bill ? validateForSML(bill) : { canSend: false, issues: [], firstBlockingItemId: null }),
    [bill],
  )
  const returnTo = useMemo(() => {
    const fromState = validateBillReturnTo((location.state as BillDetailLocationState | null)?.returnTo)
    if (fromState) return fromState
    if (!id) return null
    return validateBillReturnTo(window.sessionStorage.getItem(`billflow:return:${id}`))
  }, [id, location.state])
  const handleBack = () => {
    if (returnTo) {
      navigate(returnTo)
      return
    }
    navigate(-1)
  }

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
  const runSingleSMLSend = async (runner: () => Promise<SingleSMLSendResult | void>) => {
    if (retrying || (sendProgress.status === 'sending' && sendProgress.open)) return
    setSendProgress({ open: true, status: 'sending', docNo: null, error: null })
    try {
      const result = await runner()
      setSendProgress({
        open: true,
        status: 'success',
        docNo: result?.docNo || result?.bill?.sml_doc_no || null,
        error: null,
      })
    } catch (err) {
      const message =
        err instanceof Error && err.message
          ? err.message
          : 'ส่ง SML ไม่สำเร็จ'
      setSendProgress({
        open: true,
        status: 'error',
        docNo: null,
        error: humanizeSMLConnectionError(message),
      })
    }
  }

  const handleSendClick = () => {
    if (!(user?.role === 'admin' || user?.role === 'staff')) return
    if (retrying || (sendProgress.status === 'sending' && sendProgress.open)) return
    if (!isSMLReady(smlReadiness)) {
      toast.error('ยังส่ง SML ไม่ได้', {
        description: smlBlockedMessage(smlReadiness),
      })
      return
    }
    if (bill?.bill_type === 'purchase' || (bill?.bill_type === 'sale' && (bill?.source === 'shopee' || bill?.source === 'lazada' || bill?.source === 'tiktok'))) {
      setSendDialogOpen(true)
    } else {
      void runSingleSMLSend(() => handleRetry())
    }
  }

  const handlePurchaseConfirm = async (body: RetryBillPayload) => {
    setSendDialogOpen(false)
    await runSingleSMLSend(() => handleRetryWithOverride(body))
  }

  const handlePurchaseCreditorConfirm = async (party: Party) => {
    if (!bill || creditorUpdating) return
    setCreditorUpdating(true)
    try {
      const result = await updateBillPurchaseCreditor(bill.id, {
        party_code: party.code,
        party_name: party.name,
      })
      if (result.bill) {
        setBill(result.bill)
      } else {
        await reloadBill()
      }
      setCreditorDialogOpen(false)
      toast.success(result.message || 'อัปเดตเจ้าหนี้ใน SML แล้ว', {
        description: result.warning || result.sml_update?.log_warning || undefined,
      })
    } catch (err) {
      toast.error(
        err instanceof Error && err.message
          ? err.message
          : 'อัปเดตเจ้าหนี้ใน SML ไม่สำเร็จ',
      )
    } finally {
      setCreditorUpdating(false)
    }
  }

  const handlePrintPaymentMethodConfirm = async (paymentMethod: string, applyToEmailGroup: boolean) => {
    if (!bill || paymentUpdating) return
    setPaymentUpdating(true)
    try {
      const result = await updateBillPrintPaymentMethod(bill.id, {
        payment_method: paymentMethod,
        apply_to_email_group: applyToEmailGroup,
      })
      if (result.bill) {
        setBill(result.bill)
      }
      await reloadBill()
      setPaymentDialogOpen(false)
      toast.success(result.message || 'อัปเดตวิธีการชำระเงินสำหรับปริ้นแล้ว', {
        description: result.result?.updated_count
          ? `อัปเดต ${result.result.updated_count.toLocaleString('th-TH')} คำสั่งซื้อ`
          : undefined,
      })
    } catch (err) {
      toast.error(
        err instanceof Error && err.message
          ? err.message
          : 'อัปเดตวิธีการชำระเงินสำหรับปริ้นไม่สำเร็จ',
      )
    } finally {
      setPaymentUpdating(false)
    }
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
          onClick={handleBack}
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
    (s, i) => s + Math.max((i.qty ?? 0) * (i.price ?? 0) - (i.discount_amount ?? 0), 0),
    0,
  )
  const canSendToSML = user?.role === 'admin' || user?.role === 'staff'
  const canSend =
    bill.status === 'failed' ||
    bill.status === 'pending' ||
    bill.status === 'needs_review'
  const canEdit = canSendToSML && canSend
  const canUpdatePurchaseCreditor =
    user?.role === 'admin' &&
    bill.status === 'sent' &&
    bill.bill_type === 'purchase' &&
    (bill.source === 'shopee_shipped' || bill.source === 'lazada_email') &&
    !!bill.sml_doc_no &&
    !bill.archived_at
  const canUpdatePrintPaymentMethod =
    (user?.role === 'admin' || user?.role === 'staff') &&
    bill.status === 'sent' &&
    bill.bill_type === 'purchase' &&
    (bill.source === 'shopee_shipped' || bill.source === 'lazada_email') &&
    !!bill.sml_doc_no &&
    !bill.archived_at
  const canRepairMarketplaceEmail =
    user?.role === 'admin' &&
    (bill.source === 'shopee_shipped' || bill.source === 'lazada_email') &&
    bill.bill_type === 'purchase' &&
    !bill.archived_at

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
      <BillHeader bill={bill} onBack={handleBack} />

      {(bill.error_msg || retryError) && (
        <BillFailureCard
          errorMsg={bill.error_msg}
          retryError={retryError}
          regeneratingDocNo={regeneratingDocNo}
          onRegenerateDocNo={handleRegenerateDocNo}
          smlReadiness={smlReadiness}
        />
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
        smlReadiness={smlReadiness}
        smlReadinessLoading={smlReadinessLoading}
        canSendToSML={canSendToSML}
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

      {(canUpdatePurchaseCreditor || canUpdatePrintPaymentMethod) && (
        <div className="flex flex-wrap justify-end gap-2">
          {canUpdatePrintPaymentMethod && (
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="gap-1.5"
              onClick={() => setPaymentDialogOpen(true)}
            >
              <CreditCard className="h-4 w-4" />
              วิธีชำระเงิน
            </Button>
          )}
          {canUpdatePurchaseCreditor && (
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="gap-1.5"
              onClick={() => setCreditorDialogOpen(true)}
            >
              <UserCog className="h-4 w-4" />
              แก้เจ้าหนี้ใน SML
            </Button>
          )}
        </div>
      )}

      <section className="space-y-3">
        <div className="flex flex-wrap items-end justify-between gap-3 border-b border-border/70 pb-2">
          <div>
            <h3 className="text-sm font-semibold text-foreground">ข้อมูลประกอบการตรวจสอบ</h3>
            <p className="mt-0.5 text-xs text-muted-foreground">
              ใช้เมื่อต้องย้อนดูหลักฐานต้นฉบับ ประวัติ และข้อมูลที่ส่งเข้า SML
            </p>
          </div>
          <span className="rounded-md bg-muted px-2 py-1 text-xs text-muted-foreground">
            ข้อมูลส่วนนี้ไม่ต้องแก้ก่อนส่ง SML
          </span>
        </div>

        <div className="min-w-0 space-y-4">
          <ArtifactList
            billId={bill.id}
            billStatus={bill.status}
            billSource={bill.source}
            smlDocNo={bill.sml_doc_no ?? undefined}
            orderID={typeof bill.raw_data?.order_id === 'string' ? bill.raw_data.order_id : undefined}
            printPaymentMethod={bill.print_payment_method}
            effectivePrintPaymentMethod={bill.effective_print_payment_method}
            emailGroup={bill.email_group}
            smlPayload={bill.sml_payload}
            onReload={reloadBill}
            canRepairMarketplaceEmail={canRepairMarketplaceEmail}
          />
          <BillTimeline billId={bill.id} shopeeEvents={bill.shopee_events ?? []} />
          <SmlPayloadSection
            smlPayload={bill.sml_payload}
            smlResponse={bill.sml_response}
          />
        </div>
      </section>

      {(bill.bill_type === 'purchase' || (bill.bill_type === 'sale' && (bill.source === 'shopee' || bill.source === 'lazada' || bill.source === 'tiktok'))) && (
        <SendPurchaseDialog
          open={sendDialogOpen}
          bill={bill}
          onConfirm={handlePurchaseConfirm}
          onCancel={() => setSendDialogOpen(false)}
          onRegenerateDocNo={handleFetchLatestDocNo}
          regeneratingDocNo={refreshingDocNo}
          smlReadiness={smlReadiness}
          smlReadinessLoading={smlReadinessLoading}
        />
      )}
      <SMLSendProgressDialog
        open={sendProgress.open}
        status={sendProgress.status}
        docNo={sendProgress.docNo}
        error={sendProgress.error}
        onClose={() => setSendProgress((prev) => ({ ...prev, open: false }))}
      />
      <UpdatePurchaseCreditorDialog
        open={creditorDialogOpen}
        bill={bill}
        submitting={creditorUpdating}
        onOpenChange={setCreditorDialogOpen}
        onConfirm={handlePurchaseCreditorConfirm}
      />
      <UpdatePrintPaymentMethodDialog
        open={paymentDialogOpen}
        bill={bill}
        submitting={paymentUpdating}
        onOpenChange={setPaymentDialogOpen}
        onConfirm={handlePrintPaymentMethodConfirm}
      />
    </div>
  )
}
