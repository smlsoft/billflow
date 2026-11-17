import { useEffect, useRef, useState } from 'react'
import { Plus, Truck } from 'lucide-react'
import { toast } from 'sonner'
import client from '@/api/client'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { TagInput } from '@/components/common/TagInput'
import { ShippingCheckResult } from '@/components/shipping/ShippingCheckResult'
import {
  shippingError,
  type ShippingConfig,
  type ShippingPolicy,
  type ShippingProfile,
  type ShippingResult,
} from '@/lib/shipping-checks'
interface MailAccount {
  id: string
  name: string
  channel: string
}
interface SampleBill {
  id: string
  raw_data?: { order_id?: string }
  sml_doc_no?: string
}
const emptyConfig: ShippingConfig = { profiles: [], policies: [], revision: '' }
const fields = [
  ['name', 'ชื่อโปรไฟล์'],
  ['house', 'บ้านเลขที่'],
  ['moo', 'หมู่'],
  ['district', 'ตำบล'],
  ['amphoe', 'อำเภอ'],
  ['province', 'จังหวัด'],
  ['postcode', 'รหัสไปรษณีย์'],
] as const
const aliases = [
  ['district_aliases', 'ชื่อตำบลภาษาอังกฤษ/ชื่ออื่น'],
  ['amphoe_aliases', 'ชื่ออำเภอภาษาอังกฤษ/ชื่ออื่น'],
  ['province_aliases', 'ชื่อจังหวัดภาษาอังกฤษ/ชื่ออื่น'],
] as const
function complete(p: ShippingProfile | undefined) {
  return (
    !!p &&
    fields.every(([key]) => p[key].trim()) &&
    /^\d+(?:\/\d+)?$/.test(p.house) &&
    /^\d+$/.test(p.moo) &&
    /^\d{5}$/.test(p.postcode)
  )
}
export function ShippingCheckSettings({
  accounts,
}: {
  accounts: MailAccount[]
}) {
  const [open, setOpen] = useState(false)
  return (
    <>
      <Card className="flex flex-wrap items-center justify-between gap-3 p-4">
        <div>
          <h2 className="flex items-center gap-2 font-medium">
            <Truck className="h-4 w-4" />
            ตรวจสอบข้อมูลจัดส่ง
          </h2>
          <p className="mt-1 text-sm text-muted-foreground">
            แจ้งเตือนชื่อผู้รับหรือที่อยู่ที่ต่างจากกติกาของบริษัท
            เฉพาะบิลใหม่หลังเปิดใช้
          </p>
        </div>
        <Button variant="outline" onClick={() => setOpen(true)}>
          ตั้งค่าและทดลองตรวจ
        </Button>
      </Card>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="max-h-[90vh] max-w-4xl overflow-y-auto">
          <DialogHeader>
            <DialogTitle>ตั้งค่าตรวจสอบข้อมูลจัดส่ง</DialogTitle>
            <DialogDescription>
              สำหรับ Shopee/Lazada เท่านั้น · แจ้งเตือนใน BillFlow โดยไม่บล็อก
              SML
            </DialogDescription>
          </DialogHeader>
          {open && (
            <SettingsForm
              accounts={accounts.filter((a) =>
                ['shopee', 'lazada'].includes(a.channel),
              )}
            />
          )}
        </DialogContent>
      </Dialog>
    </>
  )
}
function SettingsForm({ accounts }: { accounts: MailAccount[] }) {
  const draftGeneration = useRef(0)
  const [config, setConfig] = useState<ShippingConfig>(emptyConfig)
  const [state, setState] = useState<'loading' | 'ready' | 'error'>('loading')
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)
  const [accountID, setAccountID] = useState(accounts[0]?.id || '')
  const [bills, setBills] = useState<SampleBill[]>([])
  const [billID, setBillID] = useState('')
  const [samplesState, setSamplesState] = useState<
    'loading' | 'ready' | 'error'
  >('loading')
  const [samplePage, setSamplePage] = useState(1)
  const [sampleRetry, setSampleRetry] = useState(0)
  const [moreSamples, setMoreSamples] = useState(false)
  const [preview, setPreview] = useState<{
    result: ShippingResult
    artifact_id?: string
    artifact_bill_id?: string
  } | null>(null)
  const [checking, setChecking] = useState(false)
  const policy = config.policies.find((p) => p.account_id === accountID) || {
    account_id: accountID,
    profile_id: '',
    recipients: [],
    enabled: false,
  }
  const profile = config.profiles.find((p) => p.id === policy.profile_id)
  const ready = complete(profile) && policy.recipients.length > 0
  const load = async () => {
    draftGeneration.current++
    setState('loading')
    setError('')
    try {
      const r = await client.get<ShippingConfig>(
        '/api/settings/shipping-checks',
      )
      setConfig({
        ...r.data,
        profiles: r.data.profiles || [],
        policies: r.data.policies || [],
      })
      setState('ready')
    } catch (e) {
      setState('error')
      setError(shippingError(e))
    }
  }
  useEffect(() => {
    void load()
  }, [])
  useEffect(() => {
    setBillID('')
    setPreview(null)
    setSamplePage(1)
    setBills([])
  }, [accountID])
  useEffect(() => {
    if (!accountID) {
      setSamplesState('ready')
      return
    }
    let alive = true
    setSamplesState('loading')
    const params = new URLSearchParams({
      email_account_id: accountID,
      bill_type: 'purchase',
      per_page: '20',
      page: String(samplePage),
    })
    client
      .get<{ data: SampleBill[]; has_more?: boolean }>(`/api/bills?${params}`)
      .then((r) => {
        if (alive) {
          setBills((prev) =>
            samplePage === 1 ? r.data.data : [...prev, ...r.data.data],
          )
          setMoreSamples(!!r.data.has_more)
          setSamplesState('ready')
        }
      })
      .catch(() => {
        if (alive) setSamplesState('error')
      })
    return () => {
      alive = false
    }
  }, [accountID, samplePage, sampleRetry])
  const update = (next: ShippingConfig) => {
    draftGeneration.current++
    setConfig(next)
    setPreview(null)
    setError('')
  }
  const updatePolicy = (next: ShippingPolicy) =>
    update({
      ...config,
      policies: [
        ...config.policies.filter((p) => p.account_id !== next.account_id),
        next,
      ],
    })
  const updateProfile = (next: ShippingProfile) =>
    update({
      ...config,
      profiles: config.profiles.map((p) => (p.id === next.id ? next : p)),
    })
  const addProfile = () => {
    const next: ShippingProfile = {
      id: `address-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
      name: '',
      house: '',
      moo: '',
      district: '',
      amphoe: '',
      province: '',
      postcode: '',
      district_aliases: [],
      amphoe_aliases: [],
      province_aliases: [],
    }
    update({
      ...config,
      profiles: [...config.profiles, next],
      policies: [
        ...config.policies.filter((p) => p.account_id !== accountID),
        { ...policy, profile_id: next.id },
      ],
    })
  }
  const save = async () => {
    setSaving(true)
    setError('')
    try {
      const r = await client.put<ShippingConfig>(
        '/api/settings/shipping-checks',
        config,
      )
      setConfig(r.data)
      setPreview(null)
      toast.success('บันทึกกติกาแล้ว ใช้กับบิลใหม่หลังจากนี้')
    } catch (e) {
      setError(shippingError(e))
    } finally {
      setSaving(false)
    }
  }
  const check = async () => {
    const generation = draftGeneration.current
    const requestAccount = accountID
    setChecking(true)
    setError('')
    setPreview(null)
    try {
      const r = await client.post('/api/settings/shipping-checks/preview', {
        account_id: accountID,
        bill_id: billID,
        config,
      })
      if (
        generation === draftGeneration.current &&
        requestAccount === accountID
      )
        setPreview(r.data)
    } catch (e) {
      setError(shippingError(e))
    } finally {
      setChecking(false)
    }
  }
  if (state === 'loading') return <p role="status">กำลังโหลดกติกา…</p>
  if (state === 'error')
    return (
      <div role="alert">
        {error}
        <Button variant="outline" onClick={() => void load()}>
          ลองใหม่
        </Button>
      </div>
    )
  if (!accounts.length)
    return <p>เพิ่มกล่องอีเมล Shopee หรือ Lazada ก่อนตั้งค่าตรวจจัดส่ง</p>
  const invalid =
    config.profiles.some((p) => !complete(p)) ||
    config.policies.some(
      (p) =>
        p.enabled &&
        (!complete(config.profiles.find((a) => a.id === p.profile_id)) ||
          !p.recipients.length),
    )
  return (
    <div className="space-y-5">
      <div className="space-y-2">
        <Label htmlFor="shipping-account">กล่องอีเมล</Label>
        <Select
          disabled={saving || checking}
          value={accountID}
          onValueChange={(v) => {
            draftGeneration.current++
            setAccountID(v)
          }}
        >
          <SelectTrigger id="shipping-account">
            <SelectValue placeholder="เลือกกล่องอีเมล" />
          </SelectTrigger>
          <SelectContent>
            {accounts.map((a) => (
              <SelectItem key={a.id} value={a.id}>
                {a.name} · {a.channel}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <fieldset
        disabled={saving || checking}
        className="space-y-4 rounded-md border p-4"
      >
        <legend className="px-1 text-sm font-medium">ที่อยู่บริษัทกลาง</legend>
        <div className="flex flex-wrap gap-2">
          <Select
            value={policy.profile_id || '__none'}
            onValueChange={(id) =>
              updatePolicy({ ...policy, profile_id: id === '__none' ? '' : id })
            }
          >
            <SelectTrigger
              aria-label="โปรไฟล์ที่อยู่"
              className="min-w-48 flex-1"
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="__none">เลือกโปรไฟล์ที่อยู่</SelectItem>
              {config.profiles.map((p) => (
                <SelectItem key={p.id} value={p.id}>
                  {p.name || 'โปรไฟล์ใหม่ (ยังไม่กรอก)'}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Button variant="outline" onClick={addProfile}>
            <Plus className="mr-1 h-4 w-4" />
            เพิ่มโปรไฟล์
          </Button>
        </div>
        {profile && (
          <>
            <p className="text-xs text-muted-foreground">
              การแก้โปรไฟล์นี้มีผลกับทุกกล่องที่เลือกใช้ สำหรับบิลใหม่เท่านั้น
              บิลที่เคยตรวจเก็บกติกาเดิมไว้
            </p>
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              {fields.map(([key, label]) => (
                <div key={key} className="space-y-1">
                  <Label htmlFor={`shipping-${key}`}>{label}</Label>
                  <Input
                    id={`shipping-${key}`}
                    maxLength={key === 'postcode' ? 5 : 160}
                    value={profile[key]}
                    onChange={(e) =>
                      updateProfile({ ...profile, [key]: e.target.value })
                    }
                  />
                </div>
              ))}
            </div>
            <details>
              <summary className="cursor-pointer text-sm">
                ชื่อพื้นที่ภาษาอังกฤษหรือชื่ออื่นที่อนุญาต
              </summary>
              <div className="mt-3 space-y-3">
                {aliases.map(([key, label]) => (
                  <div key={key}>
                    <p className="mb-1 text-sm">{label}</p>
                    <TagInput
                      value={profile[key] || []}
                      onChange={(value) =>
                        updateProfile({ ...profile, [key]: value })
                      }
                      placeholder="พิมพ์ชื่อพื้นที่แล้วกด Enter"
                    />
                  </div>
                ))}
              </div>
            </details>
            <Button
              size="sm"
              variant="outline"
              disabled={config.policies.some(
                (p) => p.profile_id === profile.id && p.enabled,
              )}
              onClick={() =>
                update({
                  ...config,
                  profiles: config.profiles.filter((p) => p.id !== profile.id),
                  policies: config.policies.map((p) =>
                    p.profile_id === profile.id ? { ...p, profile_id: '' } : p,
                  ),
                })
              }
            >
              ลบโปรไฟล์ที่ยังไม่ได้เปิดใช้
            </Button>
          </>
        )}
      </fieldset>
      <fieldset disabled={saving || checking} className="space-y-2">
        <legend className="text-sm font-medium">
          ชื่อผู้รับที่อนุญาตสำหรับกล่องนี้
        </legend>
        <TagInput
          value={policy.recipients}
          onChange={(recipients) => updatePolicy({ ...policy, recipients })}
          placeholder="กรอกชื่อผู้รับตามอีเมลแล้วกด Enter เพื่อเพิ่ม"
        />
        <p className="text-xs text-muted-foreground">
          เพิ่มเฉพาะชื่อที่บริษัทยืนยัน ไม่ใช้ชื่อบัญชีแทนชื่อผู้รับอัตโนมัติ
        </p>
      </fieldset>
      <div className="flex items-start gap-3 rounded-md border p-3">
        <Switch
          id="shipping-enabled"
          checked={policy.enabled}
          disabled={(!ready && !policy.enabled) || saving || checking}
          onCheckedChange={(enabled) => updatePolicy({ ...policy, enabled })}
        />
        <div>
          <Label htmlFor="shipping-enabled">เปิดตรวจบิลใหม่ของกล่องนี้</Label>
          <p className="mt-1 text-xs text-muted-foreground">
            {ready
              ? 'มีผลเมื่อกดบันทึก อีเมลติดตามของบิลที่เปิดตรวจไว้จะใช้กติกาที่เก็บกับบิลนั้น'
              : 'กรอกที่อยู่ครบ เลือกโปรไฟล์ และเพิ่มชื่อผู้รับก่อนเปิดใช้'}
          </p>
        </div>
      </div>
      <fieldset className="space-y-3 rounded-md border p-4">
        <legend className="px-1 text-sm font-medium">
          ทดลองตรวจก่อนเปิดใช้
        </legend>
        <p className="text-xs text-muted-foreground">
          ใช้ค่าที่กรอกอยู่กับหลักฐานบิลเดิม ไม่แก้ผลของบิล และไม่ส่ง SML
        </p>
        <Select
          value={billID || '__none'}
          onValueChange={(v) => {
            draftGeneration.current++
            setBillID(v === '__none' ? '' : v)
            setPreview(null)
          }}
        >
          <SelectTrigger aria-label="เลือกบิลตัวอย่าง">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="__none">เลือกบิลตัวอย่างจากกล่องนี้</SelectItem>
            {bills.map((b) => (
              <SelectItem key={b.id} value={b.id}>
                {b.raw_data?.order_id || b.sml_doc_no || b.id}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        {samplesState === 'loading' && (
          <p role="status" className="text-xs">
            กำลังโหลดบิลตัวอย่าง…
          </p>
        )}
        {samplesState === 'error' && (
          <p role="alert" className="text-sm">
            โหลดรายการบิลไม่สำเร็จ{' '}
            <Button
              variant="outline"
              size="sm"
              onClick={() => setSampleRetry((p) => p + 1)}
            >
              ลองใหม่
            </Button>
          </p>
        )}
        {samplesState === 'ready' && !bills.length && (
          <p className="text-sm text-muted-foreground">
            ยังไม่มีบิลซื้อในกล่องนี้ให้ทดลองตรวจ
          </p>
        )}
        {moreSamples && (
          <Button
            size="sm"
            variant="outline"
            disabled={samplesState === 'loading'}
            onClick={() => setSamplePage((p) => p + 1)}
          >
            โหลดบิลตัวอย่างเพิ่ม
          </Button>
        )}
        <Button
          variant="outline"
          disabled={!ready || !billID || checking || saving || invalid}
          onClick={() => void check()}
        >
          {checking ? 'กำลังตรวจ…' : 'ทดลองตรวจ'}
        </Button>
        {preview && (
          <ShippingCheckResult
            result={preview.result}
            artifactID={preview.artifact_id}
            artifactBillID={preview.artifact_bill_id}
          />
        )}
      </fieldset>
      {config.policies
        .filter((p) => !accounts.some((a) => a.id === p.account_id))
        .map((p) => (
          <div key={p.account_id} className="text-sm text-warning">
            มีกติกาของกล่องที่ถูกลบหรือเปลี่ยนช่องทางแล้ว{' '}
            <Button
              variant="outline"
              size="sm"
              onClick={() =>
                update({
                  ...config,
                  policies: config.policies.filter(
                    (v) => v.account_id !== p.account_id,
                  ),
                })
              }
            >
              เอากติกานี้ออก
            </Button>
          </div>
        ))}
      {error && (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}
      {invalid && (
        <p className="text-sm text-warning">
          กรอกข้อมูลโปรไฟล์ให้ครบ หรือเอาโปรไฟล์ที่ไม่ใช้แล้วออกก่อนบันทึก
        </p>
      )}
      <div className="flex flex-wrap justify-end gap-2">
        <Button
          variant="outline"
          disabled={saving || checking}
          onClick={() => void load()}
        >
          โหลดค่าที่บันทึกไว้
        </Button>
        <Button
          disabled={saving || checking || invalid}
          onClick={() => void save()}
        >
          {saving ? 'กำลังบันทึก…' : 'บันทึกกติกา'}
        </Button>
      </div>
    </div>
  )
}
