import { useEffect, useState, useCallback } from 'react'
import { useSearchParams } from 'react-router-dom'
import { MessageSquare } from 'lucide-react'

import { EmptyState } from '@/components/common/EmptyState'
import { ConversationList } from './ConversationList'
import { MessageThread } from './MessageThread'
import { CreateBillPanel } from './CreateBillPanel'
import { ExtractPreviewDialog } from './ExtractPreviewDialog'
import type { ChatConversation, ExtractedBill } from './types'

// Two-pane inbox layout. Left = conversation list (polled 30s).
// Right = active conversation thread + composer (polled 5s when open).
//
// Deep-link via ?u=<lineUserId> so admins can paste a chat URL into an email.
export default function Messages() {
  const [params, setParams] = useSearchParams()
  const selectedID = params.get('u') ?? ''
  const [selectedConv, setSelectedConv] = useState<ChatConversation | null>(null)
  const [createBillOpen, setCreateBillOpen] = useState(false)
  // When an Extract preview is approved we prefill the CreateBillPanel.
  const [billPrefill, setBillPrefill] = useState<ExtractedBill | null>(null)
  const [extractOpen, setExtractOpen] = useState(false)
  const [extractSubject, setExtractSubject] = useState<{
    messageId: string
    kind: string
  } | null>(null)

  const handleSelect = useCallback(
    (lineUserID: string) => {
      const next = new URLSearchParams(params)
      next.set('u', lineUserID)
      setParams(next, { replace: true })
    },
    [params, setParams],
  )

  // Reset prefill when CreateBill panel closes so re-opening doesn't carry
  // over previous extract data.
  useEffect(() => {
    if (!createBillOpen) {
      setBillPrefill(null)
    }
  }, [createBillOpen])

  return (
    // Layout.tsx removes default padding for /messages — we get full
    // viewport-under-topbar to work with. Inner regions (message list)
    // handle their own scroll; the outer chrome is fixed.
    <div className="flex h-full min-h-0 flex-col p-2">
      <div className="grid min-h-0 flex-1 grid-cols-[320px_minmax(0,1fr)] gap-2">
        <ConversationList
          selectedID={selectedID}
          onSelect={handleSelect}
          onSelectedConvChange={setSelectedConv}
        />

        {selectedID ? (
          <MessageThread
            lineUserID={selectedID}
            conversation={selectedConv}
            onOpenCreateBill={() => {
              setBillPrefill(null)
              setCreateBillOpen(true)
            }}
            onExtractMedia={(messageId, kind) => {
              setExtractSubject({ messageId, kind })
              setExtractOpen(true)
            }}
          />
        ) : (
          <div className="flex items-center justify-center rounded-lg border border-border bg-card">
            <EmptyState
              icon={MessageSquare}
              title="เลือกบทสนทนาทางซ้ายเพื่อเริ่ม"
              description="ลูกค้าที่ทักเข้ามาทาง LINE OA จะอยู่ทางซ้าย — คลิกเพื่อเปิดข้อความ ตอบกลับ หรือเปิดบิลขาย"
            />
          </div>
        )}
      </div>

      {selectedID && (
        <CreateBillPanel
          open={createBillOpen}
          onOpenChange={setCreateBillOpen}
          lineUserID={selectedID}
          conversation={selectedConv}
          prefill={billPrefill}
        />
      )}

      {selectedID && extractSubject && (
        <ExtractPreviewDialog
          open={extractOpen}
          onOpenChange={setExtractOpen}
          lineUserID={selectedID}
          messageId={extractSubject.messageId}
          onUseAsBill={(extracted) => {
            setBillPrefill(extracted)
            setExtractOpen(false)
            setCreateBillOpen(true)
          }}
        />
      )}
    </div>
  )
}
