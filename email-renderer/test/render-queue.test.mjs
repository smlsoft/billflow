import assert from 'node:assert/strict'
import test from 'node:test'
import { createSerialQueue } from '../render-queue.mjs'

test('runs renderer tasks one at a time in submission order', async () => {
  const queue = createSerialQueue()
  const started = []
  let active = 0
  let highestActive = 0
  let releaseFirst
  const firstGate = new Promise((resolve) => { releaseFirst = resolve })

  const first = queue.run(async () => {
    started.push('first')
    active += 1
    highestActive = Math.max(highestActive, active)
    await firstGate
    active -= 1
    return 'first result'
  })
  const second = queue.run(async () => {
    started.push('second')
    active += 1
    highestActive = Math.max(highestActive, active)
    active -= 1
    return 'second result'
  })

  await new Promise((resolve) => setImmediate(resolve))
  assert.deepEqual(started, ['first'])
  assert.deepEqual(queue.status(), { active: 1, waiting: 1 })

  releaseFirst()
  assert.deepEqual(await Promise.all([first, second]), ['first result', 'second result'])
  assert.deepEqual(started, ['first', 'second'])
  assert.equal(highestActive, 1)
  assert.deepEqual(queue.status(), { active: 0, waiting: 0 })
})

test('continues with the next renderer task after a failure', async () => {
  const queue = createSerialQueue()

  await assert.rejects(queue.run(async () => { throw new Error('render failed') }), /render failed/)
  assert.equal(await queue.run(async () => 'recovered'), 'recovered')
  assert.deepEqual(queue.status(), { active: 0, waiting: 0 })
})
