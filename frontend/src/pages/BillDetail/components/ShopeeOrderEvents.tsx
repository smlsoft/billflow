import dayjs from 'dayjs'
import { MailCheck } from 'lucide-react'
import { Card, CardContent, CardHeader } from '@/components/ui/card'
import type { ShopeeOrderEvent } from '@/types'

interface Props {
  events?: ShopeeOrderEvent[]
}

export function ShopeeOrderEvents({ events }: Props) {
  if (!events || events.length === 0) return null

  return (
    <Card className="rounded-xl border-border/70 shadow-sm">
      <CardHeader className="border-b px-4 py-3">
        <div className="flex items-center gap-2">
          <MailCheck className="h-4 w-4 text-info" />
          <h3 className="text-sm font-semibold text-foreground">ประวัติสถานะ Shopee</h3>
        </div>
      </CardHeader>
      <CardContent className="px-4 py-3">
        <div className="space-y-3">
          {events.map((event) => {
            const when = dayjs(event.email_date || event.created_at)
            return (
              <div key={event.id} className="grid gap-1 border-l-2 border-info/30 pl-3">
                <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                  <span className="text-sm font-medium text-foreground">{event.status_label}</span>
                  {event.order_id && (
                    <span className="font-mono text-xs text-muted-foreground">{event.order_id}</span>
                  )}
                  <span className="text-xs text-muted-foreground">
                    {when.isValid() ? when.format('DD/MM/YYYY HH:mm') : 'ไม่ทราบเวลา'}
                  </span>
                </div>
                {event.subject && (
                  <div className="line-clamp-2 text-xs leading-5 text-muted-foreground" title={event.subject}>
                    {event.subject}
                  </div>
                )}
              </div>
            )
          })}
        </div>
      </CardContent>
    </Card>
  )
}
