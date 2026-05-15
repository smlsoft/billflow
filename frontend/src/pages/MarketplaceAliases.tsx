import { useEffect, useMemo, useState } from 'react'
import { CheckCircle2, RefreshCw, Search, Sparkles, Tags } from 'lucide-react'
import { toast } from 'sonner'

import client from '@/api/client'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { EmptyState } from '@/components/common/EmptyState'
import { PageHeader } from '@/components/common/PageHeader'
import { Skeleton } from '@/components/ui/skeleton'
import { MapItemModal } from '@/pages/BillDetail/components/MapItemModal'
import type { CatalogMatch, MarketplaceAliasReviewGroup } from '@/types'

const SOURCE_LABEL: Record<string, string> = {
  shopee: 'Shopee',
  lazada: 'Lazada',
  tiktok: 'TikTok',
}

export default function MarketplaceAliases() {
  const [groups, setGroups] = useState<MarketplaceAliasReviewGroup[]>([])
  const [loading, setLoading] = useState(true)
  const [confirmingKey, setConfirmingKey] = useState<string | null>(null)
  const [selected, setSelected] = useState<MarketplaceAliasReviewGroup | null>(null)

  const totalItems = useMemo(() => groups.reduce((sum, g) => sum + g.item_count, 0), [groups])
  const totalBills = useMemo(() => groups.reduce((sum, g) => sum + g.bill_count, 0), [groups])

  const loadGroups = async () => {
    setLoading(true)
    try {
      const res = await client.get<{ data: MarketplaceAliasReviewGroup[] }>('/api/marketplace-aliases/review-groups', {
        params: { bill_type: 'sale' },
      })
      setGroups(res.data.data ?? [])
    } catch {
      toast.error('โหลดสินค้ารอยืนยันไม่สำเร็จ')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void loadGroups()
  }, [])

  const confirmGroup = async (group: MarketplaceAliasReviewGroup, match: CatalogMatch) => {
    setConfirmingKey(group.group_key)
    try {
      const res = await client.post<{ applied_items: number; ready_bills: number }>('/api/marketplace-aliases/confirm', {
        source: group.source,
        bill_type: group.bill_type,
        source_sku: group.source_sku,
        raw_name: group.raw_name,
        normalized_key: group.normalized_key,
        item_code: match.item_code,
        unit_code: match.unit_code,
      })
      toast.success(`ยืนยันแล้ว ${res.data.applied_items ?? 0} รายการ`)
      setSelected(null)
      await loadGroups()
    } catch {
      toast.error('ยืนยันสินค้าไม่สำเร็จ')
    } finally {
      setConfirmingKey(null)
    }
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="สินค้ารอยืนยัน"
        description="ยืนยันสินค้าจาก Shopee, TikTok, Lazada ครั้งเดียว แล้ว BillFlow จะจำเป็น alias ให้บิลถัดไป"
      />

      <div className="grid gap-3 md:grid-cols-3">
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">กลุ่มที่ต้องยืนยัน</CardTitle>
          </CardHeader>
          <CardContent className="text-2xl font-bold">{groups.length}</CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">รายการสินค้า</CardTitle>
          </CardHeader>
          <CardContent className="text-2xl font-bold">{totalItems}</CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">บิลที่เกี่ยวข้อง</CardTitle>
          </CardHeader>
          <CardContent className="flex items-center justify-between">
            <span className="text-2xl font-bold">{totalBills}</span>
            <Button variant="outline" size="sm" onClick={loadGroups} disabled={loading}>
              <RefreshCw className="mr-2 h-4 w-4" />
              รีเฟรช
            </Button>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader className="border-b">
          <CardTitle className="flex items-center gap-2 text-base">
            <Tags className="h-4 w-4 text-primary" />
            Marketplace alias review
          </CardTitle>
        </CardHeader>
        <CardContent className="p-0">
          {loading ? (
            <div className="space-y-3 p-4">
              {Array.from({ length: 5 }).map((_, i) => (
                <Skeleton key={i} className="h-24 w-full" />
              ))}
            </div>
          ) : groups.length === 0 ? (
            <div className="p-8">
              <EmptyState
                title="ไม่มีสินค้ารอยืนยัน"
                description="เมื่อมีสินค้า marketplace ที่ยังไม่มั่นใจ ระบบจะรวมกลุ่มมาให้ยืนยันที่หน้านี้"
              />
            </div>
          ) : (
            <div className="divide-y">
              {groups.map((group) => (
                <div key={group.group_key} className="grid gap-3 p-4 md:grid-cols-[minmax(0,1fr)_280px]">
                  <div className="min-w-0 space-y-2">
                    <div className="flex flex-wrap items-center gap-2">
                      <Badge variant="secondary">{SOURCE_LABEL[group.source] ?? group.source}</Badge>
                      {group.source_sku && (
                        <Badge variant="outline" className="font-mono">
                          SKU {group.source_sku}
                        </Badge>
                      )}
                      <span className="text-xs text-muted-foreground">
                        {group.item_count} รายการ · {group.bill_count} บิล
                      </span>
                    </div>
                    <div className="line-clamp-2 break-words text-sm font-medium">{group.raw_name}</div>
                    <div className="truncate text-xs text-muted-foreground">
                      key: <code>{group.normalized_key}</code>
                    </div>
                  </div>

                  <div className="flex flex-col justify-between gap-3 rounded-md border bg-muted/20 p-3">
                    {group.suggested_match ? (
                      <div className="space-y-1">
                        <div className="flex items-center gap-2 text-xs font-medium text-muted-foreground">
                          <Sparkles className="h-3.5 w-3.5" />
                          ระบบแนะนำ
                        </div>
                        <div className="text-sm font-semibold">{group.suggested_match.item_code}</div>
                        <div className="line-clamp-1 text-xs text-muted-foreground">{group.suggested_match.item_name}</div>
                        <div className="text-xs text-muted-foreground">
                          {Math.round(group.suggested_match.score * 100)}% · {group.suggested_match.unit_code || 'ไม่ระบุหน่วย'}
                        </div>
                      </div>
                    ) : (
                      <div className="flex items-center gap-2 text-sm text-muted-foreground">
                        <Search className="h-4 w-4" />
                        ยังไม่มี candidate
                      </div>
                    )}
                    <div className="flex gap-2">
                      {group.suggested_match && (
                        <Button
                          size="sm"
                          variant="outline"
                          disabled={confirmingKey === group.group_key}
                          onClick={() => confirmGroup(group, group.suggested_match as CatalogMatch)}
                        >
                          <CheckCircle2 className="mr-2 h-4 w-4" />
                          ใช้ตัวนี้
                        </Button>
                      )}
                      <Button size="sm" onClick={() => setSelected(group)}>
                        เลือกจาก SML
                      </Button>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      {selected && (
        <MapItemModal
          open={!!selected}
          rawName={selected.raw_name}
          currentCode={selected.suggested_match?.item_code ?? ''}
          currentUnit={selected.suggested_match?.unit_code ?? ''}
          currentPrice={0}
          rawNameLabel="ชื่อสินค้า marketplace"
          onPick={(code, unitCode, picked) => {
            void confirmGroup(selected, picked ?? {
              item_code: code,
              item_name: code,
              unit_code: unitCode,
              score: 1,
            })
          }}
          onClose={() => setSelected(null)}
        />
      )}
    </div>
  )
}
