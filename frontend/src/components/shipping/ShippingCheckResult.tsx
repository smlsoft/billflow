import { ShippingStatusBadge } from './ShippingCheckBadge'
import { ShippingEvidenceButton } from './ShippingEvidenceButton'
import { shippingLabels, type ShippingResult } from '@/lib/shipping-checks'
export function ShippingCheckResult({
  result,
  artifactID,
  artifactBillID,
}: {
  result: ShippingResult
  artifactID?: string
  artifactBillID?: string
}) {
  return (
    <div className="space-y-3 text-sm">
      <ShippingStatusBadge status={result.status} />
      {result.reason && (
        <p className="text-muted-foreground">{result.reason}</p>
      )}
      <div className="break-words">
        <p>
          <strong>ชื่อผู้รับจากอีเมล:</strong> {result.recipient || 'ไม่พบ'}
        </p>
        <p>
          <strong>ที่อยู่จากอีเมล:</strong> {result.address || 'ไม่พบ'}
        </p>
      </div>
      {!!result.findings?.length && (
        <div className="overflow-x-auto">
          <table className="w-full text-left text-sm">
            <caption className="sr-only">
              เปรียบเทียบข้อมูลจัดส่งกับกติกา
            </caption>
            <thead>
              <tr className="border-b">
                <th className="p-2">ข้อมูล</th>
                <th className="p-2">ที่อนุญาต</th>
                <th className="p-2">จากอีเมล</th>
                <th className="p-2">ผลตรวจ</th>
              </tr>
            </thead>
            <tbody>
              {result.findings.map((f) => (
                <tr
                  key={f.field}
                  className={
                    f.status === 'match' ? 'border-b' : 'border-b bg-warning/10'
                  }
                >
                  <th className="p-2 font-normal">{f.field}</th>
                  <td className="p-2 break-words">{f.expected}</td>
                  <td className="p-2 break-words">{f.actual || 'ไม่พบ'}</td>
                  <td className="p-2">{shippingLabels[f.status]}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {artifactID && artifactBillID && (
        <ShippingEvidenceButton
          billID={artifactBillID}
          artifactID={artifactID}
        />
      )}
    </div>
  )
}
