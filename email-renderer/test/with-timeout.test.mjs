import assert from 'node:assert/strict'
import test from 'node:test'
import { withTimeout } from '../with-timeout.mjs'

test('returns the operation result before its deadline', async () => {
  assert.equal(await withTimeout(Promise.resolve('PDF'), 50, 'timed out'), 'PDF')
})

test('rejects an operation that exceeds its deadline', async () => {
  await assert.rejects(
    withTimeout(new Promise(() => {}), 1, 'PDF render timed out'),
    /PDF render timed out/,
  )
})
