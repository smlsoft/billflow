import { useState } from 'react'
import { Send } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from '@/components/ui/dialog'
import { Label } from '@/components/ui/label'
import { PartyPicker, type Party } from '@/pages/ChannelDefaults/PartyPicker'

interface Props {
  open: boolean
  onConfirm: (partyCode: string, remark: string) => void
  onCancel: () => void
  /** Pre-fill with the channel_defaults party code when known */
  defaultPartyCode?: string
}

export function SendPurchaseDialog({ open, onConfirm, onCancel, defaultPartyCode }: Props) {
  const [party, setParty] = useState<Party | null>(null)
  const [remark, setRemark] = useState('')

  const handleConfirm = () => {
    // party_code can be empty — backend falls back to channel_defaults
    onConfirm(party?.code ?? defaultPartyCode ?? '', remark)
  }

  return (
    <Dialog open={open} onOpenChange={(v) => { if (!v) onCancel() }}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>ยืนยันการส่งใบสั่งซื้อไปยัง SML</DialogTitle>
        </DialogHeader>

        <div className="space-y-4 py-2">
          <div className="space-y-1.5">
            <Label>เจ้าหนี้ (Supplier)</Label>
            <PartyPicker
              billType="purchase"
              value={party}
              onChange={setParty}
            />
            {!party && defaultPartyCode && (
              <p className="text-[11px] text-muted-foreground">
                จะใช้ค่าเริ่มต้น: <code className="font-mono">{defaultPartyCode}</code> (จาก Channel Defaults)
              </p>
            )}
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="remark">หมายเหตุ (Remark)</Label>
            <textarea
              id="remark"
              value={remark}
              onChange={(e) => setRemark(e.target.value)}
              placeholder="หมายเหตุสำหรับ SML (ถ้ามี)"
              rows={3}
              className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring resize-none"
            />
          </div>
        </div>

        <DialogFooter className="gap-2">
          <Button type="button" variant="outline" onClick={onCancel}>
            ยกเลิก
          </Button>
          <Button type="button" onClick={handleConfirm} className="gap-2">
            <Send className="h-4 w-4" />
            ส่งไปยัง SML
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
