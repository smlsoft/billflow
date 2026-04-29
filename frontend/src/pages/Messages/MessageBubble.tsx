import { useState } from 'react'
import { AlertCircle, FileIcon, Sparkles } from 'lucide-react'
import dayjs from 'dayjs'

import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import type { ChatMessage } from './types'

interface Props {
  message: ChatMessage
  onExtract?: (messageId: string, kind: string) => void
}

// MessageBubble renders one chat row.
// - text: rounded bubble, gray for incoming / primary-tinted for outgoing
// - image: <img> via /media endpoint, click to expand
// - file: file-icon + filename + download link
// - audio: <audio controls>
// - system: muted centered note (e.g. "📄 สร้างบิลขายแล้ว")
// - failed delivery: ⚠ icon + error tooltip
// All media rows ALSO get a "🔍 สร้างบิลจากสื่อนี้" button that triggers AI extract.
export function MessageBubble({ message, onExtract }: Props) {
  const [imgExpanded, setImgExpanded] = useState(false)

  const isOutgoing = message.direction === 'outgoing'
  const isSystem = message.direction === 'system' || message.kind === 'system'

  if (isSystem) {
    return (
      <div className="my-2 flex justify-center">
        <div className="rounded-full bg-muted/60 px-3 py-1 text-[11px] text-muted-foreground">
          {message.text_content}
        </div>
      </div>
    )
  }

  const mediaURL = message.media
    ? `/api/admin/conversations/${encodeURIComponent(message.line_user_id)}/messages/${message.id}/media`
    : null

  return (
    <div className={cn('flex w-full gap-2', isOutgoing ? 'justify-end' : 'justify-start')}>
      <div className={cn('flex max-w-[75%] flex-col gap-1', isOutgoing && 'items-end')}>
        {/* Body */}
        {message.kind === 'text' && (
          <div
            className={cn(
              'whitespace-pre-wrap break-words rounded-2xl px-3 py-1.5 text-sm',
              isOutgoing
                ? 'bg-primary text-primary-foreground rounded-br-sm'
                : 'bg-accent text-foreground rounded-bl-sm',
            )}
          >
            {message.text_content}
          </div>
        )}

        {message.kind === 'image' && mediaURL && (
          <div className="overflow-hidden rounded-lg border border-border">
            <img
              src={mediaURL}
              alt={message.media?.filename ?? 'image'}
              className={cn(
                'cursor-pointer object-contain',
                imgExpanded ? 'max-h-[80vh] max-w-full' : 'max-h-60 max-w-xs',
              )}
              onClick={() => setImgExpanded((v) => !v)}
            />
          </div>
        )}

        {message.kind === 'file' && mediaURL && message.media && (
          <a
            href={mediaURL}
            target="_blank"
            rel="noreferrer"
            className="flex items-center gap-2 rounded-lg border border-border bg-card px-3 py-2 text-sm hover:bg-accent/40"
          >
            <FileIcon className="h-5 w-5 text-muted-foreground" />
            <div className="min-w-0">
              <div className="truncate font-medium">{message.media.filename}</div>
              <div className="text-[10px] text-muted-foreground">
                {(message.media.size_bytes / 1024).toFixed(0)} KB · {message.media.content_type}
              </div>
            </div>
          </a>
        )}

        {message.kind === 'audio' && mediaURL && (
          <audio controls src={mediaURL} className="h-10">
            ระบบไม่รองรับการเล่นเสียง
          </audio>
        )}

        {/* Manual AI extract trigger for media */}
        {(message.kind === 'image' || message.kind === 'file' || message.kind === 'audio') &&
          !isOutgoing &&
          onExtract && (
            <Button
              variant="outline"
              size="sm"
              className="h-7 gap-1 px-2 text-[11px]"
              onClick={() => onExtract(message.id, message.kind)}
            >
              <Sparkles className="h-3 w-3" />
              สร้างบิลจากสื่อนี้
            </Button>
          )}

        {/* Footer: timestamp + delivery status */}
        <div className="flex items-center gap-1.5 px-1 text-[10px] text-muted-foreground">
          <span>{dayjs(message.created_at).format('HH:mm')}</span>
          {isOutgoing && message.delivery_status === 'failed' && (
            <span
              className="flex items-center gap-0.5 text-destructive"
              title={message.delivery_error || 'ส่งไม่สำเร็จ'}
            >
              <AlertCircle className="h-3 w-3" />
              ส่งไม่สำเร็จ
            </span>
          )}
          {isOutgoing && message.delivery_status === 'pending' && <span>กำลังส่ง…</span>}
        </div>
      </div>
    </div>
  )
}
