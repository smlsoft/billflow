import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import ts from 'typescript'
const compile = (path) => ts.transpileModule(readFileSync(new URL(path, import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const url = (js) => `data:text/javascript;base64,${Buffer.from(js).toString('base64')}`
const dependency = url(compile('../src/lib/shopeeBill.ts'))
const compiled = compile('../src/pages/BillDetail/utils/validation.ts').replace("'@/lib/shopeeBill'", JSON.stringify(dependency)).replace('"@/lib/shopeeBill"', JSON.stringify(dependency))
const { validateForSML } = await import(url(compiled))
const validBill = { items: [{ id: 'item', item_code: 'PRODUCT', unit_code: 'PCS', mapped: true, qty: 1, price: 99 }] }
test('single/bulk validation refuses a bill until server reports a manual pass', () => {
  const result = validateForSML({ ...validBill, shipping_send_check: { required: true, can_send: false, reason: 'ต้องกดตรวจ' } })
  assert.equal(result.canSend, false)
  assert.equal(result.issues[0].kind, 'shipping_check')
  assert.equal(result.issues[0].message, 'ต้องกดตรวจ')
  assert.equal(validateForSML({ ...validBill, shipping_send_check: { required: true, can_send: true } }).canSend, true)
})
test('ordinary instances keep existing validation and shipping gate combines with missing items', () => {
  assert.equal(validateForSML(validBill).canSend, true)
  const result = validateForSML({ items: [], shipping_send_check: { required: true, can_send: false } })
  assert.deepEqual(result.issues.map((i) => i.kind), ['shipping_check', 'no_items'])
})
