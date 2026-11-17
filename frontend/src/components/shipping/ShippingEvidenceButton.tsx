import { useEffect, useState } from 'react'
import client from '@/api/client'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from '@/components/ui/dialog'
export function ShippingEvidenceButton({
  billID,
  artifactID,
}: {
  billID: string
  artifactID: string
}) {
  const [open, setOpen] = useState(false)
  const [src, setSrc] = useState('')
  const [failed, setFailed] = useState(false)
  useEffect(() => {
    if (!open) return
    let alive = true
    let url = ''
    setSrc('')
    setFailed(false)
    client
      .get(`/api/bills/${billID}/artifacts/${artifactID}/preview`, {
        responseType: 'blob',
      })
      .then((r) => {
        if (!alive) return
        url = URL.createObjectURL(new Blob([r.data], { type: 'text/html' }))
        setSrc(url)
      })
      .catch(() => {
        if (alive) setFailed(true)
      })
    return () => {
      alive = false
      if (url) URL.revokeObjectURL(url)
    }
  }, [open, billID, artifactID])
  return (
    <>
      <Button size="sm" variant="outline" onClick={() => setOpen(true)}>
        เปิดหลักฐานอีเมล
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="max-w-5xl">
          <DialogHeader>
            <DialogTitle>หลักฐานอีเมลจัดส่ง</DialogTitle>
            <DialogDescription>
              แสดงหลักฐานต้นทางในกรอบที่ปิดการทำงานของสคริปต์
            </DialogDescription>
          </DialogHeader>
          {failed ? (
            <p role="alert">เปิดหลักฐานไม่สำเร็จ กรุณาปิดแล้วลองใหม่</p>
          ) : src ? (
            <iframe
              title="หลักฐานอีเมลจัดส่ง"
              src={src}
              sandbox=""
              className="h-[70vh] w-full border bg-white"
            />
          ) : (
            <p role="status">กำลังโหลด…</p>
          )}
        </DialogContent>
      </Dialog>
    </>
  )
}
