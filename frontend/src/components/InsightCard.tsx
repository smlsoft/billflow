import { Sparkles } from 'lucide-react'
import dayjs from 'dayjs'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import type { DailyInsight } from '@/types'

export default function InsightCard({ insight }: { insight: DailyInsight | null }) {
  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-3">
        <CardTitle className="flex items-center gap-2 text-sm font-semibold text-foreground">
          <Sparkles className="h-4 w-4 text-primary" />
          AI Insight ประจำวัน
        </CardTitle>
        {insight && (
          <span className="text-xs text-muted-foreground">
            {dayjs(insight.date).format('DD/MM/YYYY')}
          </span>
        )}
      </CardHeader>
      <CardContent>
        {insight ? (
          <p className="whitespace-pre-line text-sm leading-relaxed text-foreground">
            {insight.insight}
          </p>
        ) : (
          <p className="text-sm italic text-muted-foreground">
            ยังไม่มี insight วันนี้ — กด "สร้าง AI Insight" เพื่อสร้างรายงาน
          </p>
        )}
      </CardContent>
    </Card>
  )
}
