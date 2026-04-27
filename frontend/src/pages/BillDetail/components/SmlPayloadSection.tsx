import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { JsonViewer } from '@/components/common/JsonViewer'

interface Props {
  smlPayload?: Record<string, unknown> | null
  smlResponse?: Record<string, unknown> | null
}

export function SmlPayloadSection({ smlPayload, smlResponse }: Props) {
  if (!smlPayload && !smlResponse) return null

  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="text-sm font-semibold">SML Request / Response</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3 pt-0">
        {smlPayload && (
          <JsonViewer
            title="sml_payload (ข้อมูลที่ส่งไป SML)"
            data={smlPayload}
            defaultOpen={false}
          />
        )}
        {smlResponse && (
          <JsonViewer
            title="sml_response (ผลตอบกลับจาก SML)"
            data={smlResponse}
            defaultOpen={false}
          />
        )}
      </CardContent>
    </Card>
  )
}
