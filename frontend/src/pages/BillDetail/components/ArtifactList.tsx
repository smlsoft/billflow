import { Eye, Download, Paperclip } from 'lucide-react'
import dayjs from 'dayjs'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { useArtifacts, openArtifact } from '../hooks/useArtifacts'
import { KIND_META, fmtSize } from '../utils/formatters'

interface Props {
  billId: string
}

export function ArtifactList({ billId }: Props) {
  const { items, loading } = useArtifacts(billId)

  if (loading || items.length === 0) return null

  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="flex items-center gap-2 text-sm font-semibold">
          <Paperclip className="h-4 w-4 text-muted-foreground" />
          หลักฐานต้นฉบับ ({items.length})
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-1 pt-0">
        {/* Audit trail info banner */}
        <div className="mb-3 rounded-md bg-muted/40 px-3 py-2 text-xs text-muted-foreground leading-relaxed">
          ℹ️ เก็บไฟล์{' '}
          <strong>ต้นฉบับ</strong>{' '}
          ที่ระบบใช้สร้างบิลนี้ — เปิดดู/ดาวน์โหลดได้เพื่อย้อนตรวจว่าข้อมูล
          (สินค้า, จำนวน, ราคา) มาจากที่ไหน ไม่ได้สร้างขึ้นเอง · ทุกไฟล์ถูก
          hash ด้วย SHA-256 ป้องกันการแก้ไข
        </div>

        {items.map((a) => {
          const meta = KIND_META[a.kind] ?? { icon: '📎', label: a.kind, desc: '' }
          const previewable =
            a.content_type === 'application/pdf' ||
            (a.content_type ?? '').startsWith('image/') ||
            (a.content_type ?? '').startsWith('text/') ||
            a.content_type === 'application/json'
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
                    onClick={() => openArtifact(billId, a.id, a.filename, 'preview')}
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
  )
}
