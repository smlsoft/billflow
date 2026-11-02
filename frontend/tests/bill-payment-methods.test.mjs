import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import ts from 'typescript'

// Compile the real pure TS module with the project's existing compiler so
// these tests also run on Node versions without native type stripping.
const source = readFileSync(new URL('../src/lib/bill-payment-methods.ts', import.meta.url), 'utf8')
const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext } }).outputText
const { billPaymentMethods } = await import(`data:text/javascript;base64,${Buffer.from(compiled).toString('base64')}`)
const fallback = ['TT9630', 'โอน Kbank']
const rows = [
  { channel: 'shopee_shipped', bill_type: 'purchase', print_policy: { payment_methods: ['TT5128', ' TT9610 ', 'TT9610'] } },
  { channel: 'lazada_email', bill_type: 'purchase', print_policy: { payment_methods: ['TT8456', 'TT4553', 'TT3132', 'TT5432'] } },
  { channel: 'manual', bill_type: 'purchase', print_policy: { payment_methods: ['MANUAL'] } },
  { channel: 'shopee_shipped', bill_type: 'sale', print_policy: { payment_methods: ['SALE'] } },
]

test('new configured cards appear even when the current bill page is empty', () => {
  assert.deepEqual(billPaymentMethods(rows, '', [], '', fallback), ['TT5128', 'TT9610', 'TT8456', 'TT4553', 'TT3132', 'TT5432'])
})

test('channel selection limits configured options without reintroducing defaults', () => {
  assert.deepEqual(billPaymentMethods(rows, 'shopee_shipped', [], '', fallback), ['TT5128', 'TT9610'])
  assert.deepEqual(billPaymentMethods(rows, 'lazada_email', [], '', fallback), ['TT8456', 'TT4553', 'TT3132', 'TT5432'])
})

test('historical effective methods and the active filter stay searchable', () => {
  const bills = [
    { effective_print_payment_method: 'TT-OLD', print_payment_method: 'TT-OVERRIDDEN' },
    { print_payment_method: ' โอน BBL ' },
    { print_payment_method: 'TT5128' },
  ]
  assert.deepEqual(billPaymentMethods(rows, 'shopee_shipped', bills, 'TT-URL', fallback), ['TT5128', 'TT9610', 'TT-OLD', 'โอน BBL', 'TT-URL'])
})

test('fallback applies only where channel policy is absent or empty', () => {
  assert.deepEqual(billPaymentMethods(null, '', [], '', fallback), fallback)
  assert.deepEqual(billPaymentMethods([], 'shopee_shipped', [], '', fallback), fallback)
  assert.deepEqual(billPaymentMethods([{channel:'lazada_email',bill_type:'purchase',print_policy:{payment_methods:['  ']}}], 'lazada_email', [], '', fallback), fallback)
})

test('replacing server config replaces options and removes unused old cards', () => {
  const updated = [{channel:'shopee_shipped',bill_type:'purchase',print_policy:{payment_methods:['TT-NEW']}}]
  assert.deepEqual(billPaymentMethods(updated, 'shopee_shipped', [], '', fallback), ['TT-NEW'])
})
