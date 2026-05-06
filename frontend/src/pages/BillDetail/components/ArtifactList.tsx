import { useState } from 'react'
import { Eye, Download, Paperclip, X } from 'lucide-react'
import dayjs from 'dayjs'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { useArtifacts, openArtifact } from '../hooks/useArtifacts'
import { KIND_META, fmtSize } from '../utils/formatters'
import api from '@/api/client'

interface Props {
  billId: string
}

// EmailPreviewModal renders HTML email content in a sandboxed iframe so the
// browser treats it as a rendered email (layout, images, Thai text) instead of
// a raw text dump in a new tab.
function EmailPreviewModal({
  billId,
  artId,
  filename,
  onClose,
}: {
  billId: string
  artId: string
  filename: string
  onClose: () => void
}) {
  const [src, setSrc] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)

  // Fetch once on mount — blob URL lives until modal closes.
  useState(() => {
    api
      .get(`/api/bills/${billId}/artifacts/${artId}/preview`, { responseType: 'blob' })
      .then((res) => {
        const ct = (res.headers['content-type'] ?? '').toString() || 'text/html; charset=utf-8'
        const blob = new Blob([res.data as Blob], { type: ct })
        setSrc(URL.createObjectURL(blob))
      })
      .finally(() => setLoading(false))
    return () => {}
  })

  const handleClose = () => {
    if (src) URL.revokeObjectURL(src)
    onClose()
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4"
      onClick={handleClose}
    >
      <div
        className="relative flex h-[90vh] w-full max-w-4xl flex-col rounded-lg bg-white shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between border-b px-4 py-2">
          <span className="text-sm font-medium text-foreground">{filename}</span>
          <button
            type="button"
            onClick={handleClose}
            title="ปิด"
            className="rounded p-1 text-muted-foreground hover:bg-muted hover:text-foreground"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
        <div className="flex-1 overflow-hidden">
          {loading && (
            <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
              กำลังโหลด...
            </div>
          )}
          {src && (
            <iframe
              src={src}
              title={filename}
              className="h-full w-full border-0"
              sandbox="allow-same-origin allow-popups"
              referrerPolicy="no-referrer"
            />
          )}
        </div>
      </div>
    </div>
  )
}

export function ArtifactList({ billId }: Props) {
  const { items, loading } = useArtifacts(billId)
  const [previewArt, setPreviewArt] = useState<{ id: string; filename: string; contentType: string } | null>(null)

  if (loading || items.length === 0) return null

  return (
    <>
      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="flex items-center gap-2 text-sm font-semibold">
            <Paperclip className="h-4 w-4 text-muted-foreground" />
            หลักฐานต้นฉบับ ({items.length})
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-1 pt-0">
          <div className="mb-3 rounded-md bg-muted/40 px-3 py-2 text-xs text-muted-foreground leading-relaxed">
            ℹ️ เก็บไฟล์{' '}
            <strong>ต้นฉบับ</strong>{' '}
            ที่ระบบใช้สร้างบิลนี้ — เปิดดู/ดาวน์โหลดได้เพื่อย้อนตรวจว่าข้อมูล
            (สินค้า, จำนวน, ราคา) มาจากที่ไหน ไม่ได้สร้างขึ้นเอง · ทุกไฟล์ถูก
            hash ด้วย SHA-256 ป้องกันการแก้ไข
          </div>

          {items.map((a) => {
            const meta = KIND_META[a.kind] ?? { icon: '📎', label: a.kind, desc: '' }
            const ct = a.content_type ?? ''
            const isHtml = ct.startsWith('text/html') || a.kind === 'email_html' || a.kind === 'email_text'
            const previewable =
              ct === 'application/pdf' ||
              ct.startsWith('image/') ||
              ct.startsWith('text/') ||
              ct === 'application/json'

            const handlePreview = () => {
              if (isHtml) {
                setPreviewArt({ id: a.id, filename: a.filename, contentType: ct })
              } else {
                openArtifact(billId, a.id, a.filename, 'preview')
              }
            }

            return (
              <div
                key={a.id}
                className="flex items-start gap-3 border-b border-border/50 py-3 last:border-0"
              >
                <span className="text-xl leading-snug">{meta.icon}</span>
                <div className="flex-1 min-w-0">
                  <div className="font-medium text-sm break-words">{meta.label}</div>
                  {meta.desc && (
                    <div className="mt-0.5 text-xs text-muted-foreground leading-snug">
                      {meta.desc}
                    </div>
                  )}
                  <div className="mt-1 font-mono text-[11px] text-muted-foreground/70">
                    {a.filename} · {fmtSize(a.size_bytes)} ·{' '}
                    {dayjs(a.created_at).format('DD/MM/YY HH:mm')}
                  </div>
                </div>
                <div className="flex shrink-0 gap-2">
                  {previewable && (
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      className="h-8 gap-1.5"
                      title={a.sha256 ? `SHA256: ${a.sha256.slice(0, 16)}…` : ''}
                      onClick={handlePreview}
                    >
                      <Eye className="h-3.5 w-3.5" />
                      ดู
                    </Button>
                  )}
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    className="h-8 gap-1.5"
                    title={a.sha256 ? `SHA256: ${a.sha256.slice(0, 16)}…` : ''}
                    onClick={() => openArtifact(billId, a.id, a.filename, 'download')}
                  >
                    <Download className="h-3.5 w-3.5" />
                    ดาวน์โหลด
                  </Button>
                </div>
              </div>
            )
          })}
        </CardContent>
      </Card>

      {previewArt && (
        <EmailPreviewModal
          billId={billId}
          artId={previewArt.id}
          filename={previewArt.filename}
          onClose={() => setPreviewArt(null)}
        />
      )}
    </>
  )
}
