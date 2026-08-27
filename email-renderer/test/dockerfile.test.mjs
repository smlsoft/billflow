import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'

const rendererDir = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')

test('copies all local renderer ES modules into the production image', () => {
  const dockerfile = fs.readFileSync(path.join(rendererDir, 'Dockerfile'), 'utf8')
  assert.match(dockerfile, /^COPY \*\.mjs \.\/$/m)
})
