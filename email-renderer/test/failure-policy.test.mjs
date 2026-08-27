import assert from 'node:assert/strict'
import test from 'node:test'
import { requiresRendererRestart } from '../failure-policy.mjs'

test('restarts when Chromium cannot spawn because the PID budget is exhausted', () => {
  assert.equal(
    requiresRendererRestart(new Error('browserType.launch: Failed to launch: spawn chrome-headless-shell EAGAIN')),
    true,
  )
  assert.equal(requiresRendererRestart(new Error('sh: Cannot fork')), true)
})

test('does not restart for a normal rendering error', () => {
  assert.equal(requiresRendererRestart(new Error('ข้อมูล HTML มีขนาดใหญ่เกินกำหนด')), false)
})
