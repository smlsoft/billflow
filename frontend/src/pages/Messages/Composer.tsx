import { useEffect, useRef, useState, type KeyboardEvent } from 'react'
import { MessageSquare, Send } from 'lucide-react'

import { Button } from '@/components/ui/button'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { Textarea } from '@/components/ui/textarea'
import client from '@/api/client'

interface QuickReply {
  id: string
  label: string
  body: string
  sort_order: number
}

interface Props {
  disabled?: boolean
  onSend: (text: string) => Promise<void>
}

// Composer is the sticky bottom textarea + Send button + quick-replies popover.
// Cmd/Ctrl+Enter sends; plain Enter adds a newline. The 💬 button opens a
// list of admin-defined templates (Phase 4.4); clicking one fills the textarea.
export function Composer({ disabled, onSend }: Props) {
  const [text, setText] = useState('')
  const [sending, setSending] = useState(false)
  const [quickReplies, setQuickReplies] = useState<QuickReply[]>([])
  const [qrOpen, setQrOpen] = useState(false)
  const textareaRef = useRef<HTMLTextAreaElement | null>(null)

  // Lazy-fetch templates on first popover open.
  useEffect(() => {
    if (!qrOpen || quickReplies.length > 0) return
    client
      .get<{ data: QuickReply[] }>('/api/admin/quick-replies')
      .then((res) => setQuickReplies(res.data.data ?? []))
      .catch(() => setQuickReplies([]))
  }, [qrOpen, quickReplies.length])

  const submit = async () => {
    const trimmed = text.trim()
    if (!trimmed || sending) return
    setSending(true)
    try {
      await onSend(trimmed)
      setText('')
    } finally {
      setSending(false)
    }
  }

  const handleKey = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
      e.preventDefault()
      void submit()
    }
  }

  const insertTemplate = (body: string) => {
    setText((prev) => (prev ? prev + '\n' + body : body))
    setQrOpen(false)
    // Focus textarea after popover closes so admin can edit immediately.
    setTimeout(() => textareaRef.current?.focus(), 50)
  }

  return (
    <div className="flex items-end gap-2 border-t border-border bg-card p-3">
      <Popover open={qrOpen} onOpenChange={setQrOpen}>
        <PopoverTrigger asChild>
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="h-[60px] gap-1.5 px-3"
            title="ใช้ template (quick reply)"
          >
            <MessageSquare className="h-4 w-4" />
          </Button>
        </PopoverTrigger>
        <PopoverContent className="w-72 p-0" align="start" side="top">
          {quickReplies.length === 0 ? (
            <div className="p-3 text-xs text-muted-foreground">
              ยังไม่มี template — เพิ่มใน /settings/messages
            </div>
          ) : (
            <ul className="max-h-72 divide-y divide-border overflow-y-auto">
              {quickReplies.map((q) => (
                <li key={q.id}>
                  <button
                    type="button"
                    onClick={() => insertTemplate(q.body)}
                    className="flex w-full flex-col items-start gap-0.5 px-3 py-2 text-left text-xs hover:bg-accent/40"
                  >
                    <span className="font-medium">{q.label}</span>
                    <span className="line-clamp-2 text-muted-foreground">{q.body}</span>
                  </button>
                </li>
              ))}
            </ul>
          )}
        </PopoverContent>
      </Popover>

      <Textarea
        ref={textareaRef}
        value={text}
        onChange={(e) => setText(e.target.value)}
        onKeyDown={handleKey}
        disabled={disabled || sending}
        placeholder="พิมพ์ข้อความ… (Cmd/Ctrl+Enter เพื่อส่ง)"
        rows={2}
        className="min-h-[60px] resize-none text-sm"
      />
      <Button
        type="button"
        onClick={submit}
        disabled={disabled || sending || !text.trim()}
        className="h-[60px] gap-1.5 px-4"
      >
        <Send className="h-4 w-4" />
        ส่ง
      </Button>
    </div>
  )
}
