import { useEffect, useState } from 'react'
import { Check, Pencil, Plus, Trash2, X } from 'lucide-react'
import toast from 'react-hot-toast'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import LearningProgress from '@/components/LearningProgress'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'
import { EmptyState } from '@/components/common/EmptyState'
import { PageHeader } from '@/components/common/PageHeader'
import { Skeleton } from '@/components/ui/skeleton'
import client from '@/api/client'
import type { Mapping, MappingStats } from '@/types'

interface MappingDraft {
  raw_name: string
  item_code: string
  unit_code: string
}

const emptyDraft: MappingDraft = { raw_name: '', item_code: '', unit_code: '' }

export default function Mappings() {
  const [mappings, setMappings] = useState<Mapping[]>([])
  const [stats, setStats] = useState<MappingStats | null>(null)
  const [loading, setLoading] = useState(true)
  const [editId, setEditId] = useState<string | null>(null)
  const [editDraft, setEditDraft] = useState<MappingDraft>(emptyDraft)
  const [newMapping, setNewMapping] = useState<MappingDraft>(emptyDraft)
  const [adding, setAdding] = useState(false)
  const [deleteId, setDeleteId] = useState<string | null>(null)

  const fetchAll = async () => {
    setLoading(true)
    try {
      const [mRes, sRes] = await Promise.all([
        client.get<{ data: Mapping[] }>('/api/mappings'),
        client.get<MappingStats>('/api/mappings/stats'),
      ])
      setMappings(mRes.data.data ?? [])
      setStats(sRes.data)
    } catch {
      toast.error('โหลด mapping ไม่สำเร็จ')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    fetchAll()
  }, [])

  const startEdit = (m: Mapping) => {
    setEditId(m.id)
    setEditDraft({ raw_name: m.raw_name, item_code: m.item_code, unit_code: m.unit_code })
  }

  const handleSave = async (id: string) => {
    if (
      !editDraft.raw_name.trim() ||
      !editDraft.item_code.trim() ||
      !editDraft.unit_code.trim()
    ) {
      toast.error('กรอกครบทั้ง 3 ช่องก่อนบันทึก')
      return
    }
    try {
      await client.put(`/api/mappings/${id}`, editDraft)
      setEditId(null)
      fetchAll()
      toast.success('บันทึกสำเร็จ')
    } catch {
      toast.error('บันทึกไม่สำเร็จ')
    }
  }

  const handleDelete = async () => {
    if (!deleteId) return
    try {
      await client.delete(`/api/mappings/${deleteId}`)
      fetchAll()
      toast.success('ลบสำเร็จ')
    } catch {
      toast.error('ลบไม่สำเร็จ')
    }
  }

  const handleAdd = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!newMapping.raw_name || !newMapping.item_code || !newMapping.unit_code) return
    setAdding(true)
    try {
      await client.post('/api/mappings', newMapping)
      setNewMapping(emptyDraft)
      fetchAll()
      toast.success('เพิ่ม mapping สำเร็จ')
    } catch {
      toast.error('เพิ่ม mapping ไม่สำเร็จ')
    } finally {
      setAdding(false)
    }
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="Mapping สินค้า"
        description="จัดการการจับคู่ชื่อสินค้ากับรหัสใน SML — ระบบจะเรียนรู้ทุกครั้งที่คุณยืนยัน (F1 Learning)"
      />

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-[1fr_320px]">
        {/* Table */}
        <div className="overflow-hidden rounded-lg border border-border bg-card">
          <Table>
            <TableHeader>
              <TableRow className="bg-muted/40 hover:bg-muted/40">
                <TableHead className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                  ชื่อดิบ
                </TableHead>
                <TableHead className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                  Item Code
                </TableHead>
                <TableHead className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                  หน่วย
                </TableHead>
                <TableHead className="text-center text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                  แหล่ง
                </TableHead>
                <TableHead className="text-center text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                  ใช้
                </TableHead>
                <TableHead className="w-[120px] text-center text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                  จัดการ
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {loading ? (
                Array.from({ length: 6 }).map((_, i) => (
                  <TableRow key={i}>
                    <TableCell colSpan={6}>
                      <Skeleton className="h-5 w-full" />
                    </TableCell>
                  </TableRow>
                ))
              ) : mappings.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={6} className="py-12">
                    <EmptyState
                      title="ยังไม่มี mapping"
                      description="ระบบจะเรียนรู้เมื่อคุณตรวจสอบบิล หรือเพิ่มเองจากฟอร์มด้านขวา"
                    />
                  </TableCell>
                </TableRow>
              ) : (
                mappings.map((m) => {
                  const isEditing = editId === m.id
                  return (
                    <TableRow key={m.id} className="h-12">
                      <TableCell>
                        {isEditing ? (
                          <Input
                            value={editDraft.raw_name}
                            onChange={(e) =>
                              setEditDraft((d) => ({ ...d, raw_name: e.target.value }))
                            }
                            className="h-8"
                          />
                        ) : (
                          <span className="text-sm">{m.raw_name}</span>
                        )}
                      </TableCell>
                      <TableCell>
                        {isEditing ? (
                          <Input
                            value={editDraft.item_code}
                            onChange={(e) =>
                              setEditDraft((d) => ({ ...d, item_code: e.target.value }))
                            }
                            className="h-8 font-mono text-xs"
                            autoFocus
                          />
                        ) : (
                          <span className="font-mono text-xs font-medium">{m.item_code}</span>
                        )}
                      </TableCell>
                      <TableCell>
                        {isEditing ? (
                          <Input
                            value={editDraft.unit_code}
                            onChange={(e) =>
                              setEditDraft((d) => ({ ...d, unit_code: e.target.value }))
                            }
                            className="h-8 w-20"
                          />
                        ) : (
                          <span className="text-sm text-muted-foreground">
                            {m.unit_code || '—'}
                          </span>
                        )}
                      </TableCell>
                      <TableCell className="text-center">
                        <Badge
                          variant={m.source === 'ai_learned' ? 'default' : 'secondary'}
                          className={
                            m.source === 'ai_learned'
                              ? 'bg-success/15 text-success hover:bg-success/20'
                              : ''
                          }
                        >
                          {m.source === 'ai_learned' ? 'AI เรียนรู้' : 'manual'}
                        </Badge>
                      </TableCell>
                      <TableCell className="text-center text-xs tabular-nums text-muted-foreground">
                        {m.usage_count}
                      </TableCell>
                      <TableCell>
                        <div className="flex items-center justify-center gap-1">
                          {isEditing ? (
                            <>
                              <Button
                                size="icon"
                                variant="default"
                                className="h-7 w-7"
                                onClick={() => handleSave(m.id)}
                                title="บันทึก"
                              >
                                <Check className="h-3.5 w-3.5" />
                              </Button>
                              <Button
                                size="icon"
                                variant="outline"
                                className="h-7 w-7"
                                onClick={() => setEditId(null)}
                                title="ยกเลิก"
                              >
                                <X className="h-3.5 w-3.5" />
                              </Button>
                            </>
                          ) : (
                            <>
                              <Button
                                size="icon"
                                variant="ghost"
                                className="h-7 w-7"
                                onClick={() => startEdit(m)}
                                title="แก้ไข"
                              >
                                <Pencil className="h-3.5 w-3.5" />
                              </Button>
                              <Button
                                size="icon"
                                variant="ghost"
                                className="h-7 w-7 text-muted-foreground hover:text-destructive"
                                onClick={() => setDeleteId(m.id)}
                                title="ลบ"
                              >
                                <Trash2 className="h-3.5 w-3.5" />
                              </Button>
                            </>
                          )}
                        </div>
                      </TableCell>
                    </TableRow>
                  )
                })
              )}
            </TableBody>
          </Table>
        </div>

        {/* Sidebar */}
        <div className="space-y-4">
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="flex items-center gap-2 text-sm font-semibold">
                <Plus className="h-4 w-4" />
                เพิ่ม Mapping ใหม่
              </CardTitle>
            </CardHeader>
            <CardContent>
              <form onSubmit={handleAdd} className="space-y-3">
                <div className="space-y-1.5">
                  <Label htmlFor="m-raw">ชื่อดิบ</Label>
                  <Input
                    id="m-raw"
                    placeholder="ชื่อสินค้าจาก LINE / Email"
                    value={newMapping.raw_name}
                    onChange={(e) =>
                      setNewMapping((p) => ({ ...p, raw_name: e.target.value }))
                    }
                    required
                  />
                </div>
                <div className="space-y-1.5">
                  <Label htmlFor="m-code">Item Code (SML)</Label>
                  <Input
                    id="m-code"
                    placeholder="เช่น CEM001"
                    className="font-mono"
                    value={newMapping.item_code}
                    onChange={(e) =>
                      setNewMapping((p) => ({ ...p, item_code: e.target.value }))
                    }
                    required
                  />
                </div>
                <div className="space-y-1.5">
                  <Label htmlFor="m-unit">หน่วย</Label>
                  <Input
                    id="m-unit"
                    placeholder="เช่น ถุง, เส้น"
                    value={newMapping.unit_code}
                    onChange={(e) =>
                      setNewMapping((p) => ({ ...p, unit_code: e.target.value }))
                    }
                    required
                  />
                </div>
                <Button type="submit" disabled={adding} className="w-full">
                  {adding ? 'กำลังเพิ่ม…' : 'เพิ่ม Mapping'}
                </Button>
              </form>
            </CardContent>
          </Card>

          {stats && <LearningProgress stats={stats} />}
        </div>
      </div>

      <ConfirmDialog
        open={deleteId !== null}
        onOpenChange={(o) => !o && setDeleteId(null)}
        title="ลบ mapping นี้?"
        description="หลังลบแล้วระบบจะไม่ใช้ mapping นี้อีก แต่จะกลับมาเรียนรู้ใหม่หากคุณ map ซ้ำในบิลใดบิลหนึ่ง"
        variant="destructive"
        confirmLabel="ลบ"
        onConfirm={handleDelete}
      />
    </div>
  )
}
