import assert from 'node:assert/strict'
import test from 'node:test'
import { isPIDBudgetAtRisk, parsePIDBudget } from '../pid-budget.mjs'

test('detects a renderer approaching its cgroup PID limit', () => {
  assert.deepEqual(parsePIDBudget('128\n', '128\n'), { current: 128, max: 128 })
  assert.equal(isPIDBudgetAtRisk(parsePIDBudget('103', '128')), true)
  assert.equal(isPIDBudgetAtRisk(parsePIDBudget('30', '128')), false)
})

test('ignores unlimited or malformed cgroup PID values', () => {
  assert.equal(parsePIDBudget('10', 'max'), null)
  assert.equal(parsePIDBudget('not-a-number', '128'), null)
  assert.equal(isPIDBudgetAtRisk(null), false)
})
