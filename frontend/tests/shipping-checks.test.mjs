import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import ts from 'typescript'
const source = readFileSync(new URL('../src/lib/shipping-checks.ts', import.meta.url), 'utf8')
const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext } }).outputText
const { shippingSummary, shippingLabels } = await import(`data:text/javascript;base64,${Buffer.from(compiled).toString('base64')}`)
test('historical bills are unchecked, not matching', () => {
  assert.equal(shippingSummary({}), undefined)
  assert.equal(shippingSummary(null), undefined)
})
test('a later matching event keeps the warning visible in the summary', () => {
  const summary = shippingSummary({ shipping_check_summary: { status: 'match', attention: true, has_warning: true } })
  assert.equal(summary.status, 'match')
  assert.equal(summary.attention, true)
  assert.equal(shippingLabels.unverifiable, 'ตรวจสอบไม่ได้')
})

test('manual check request id works without the secure-context randomUUID API', async () => {
  const { newShippingCheckRequestID } = await import(`data:text/javascript;base64,${Buffer.from(compiled).toString('base64')}`)
  const first = newShippingCheckRequestID()
  assert.match(first, /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/)
  assert.notEqual(first, newShippingCheckRequestID())
})
