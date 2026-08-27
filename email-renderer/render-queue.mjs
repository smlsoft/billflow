// Playwright runs inside one small, isolated container. Keep render requests
// strictly serial so Chromium sessions cannot compete for its PID budget.
export function createSerialQueue() {
  let tail = Promise.resolve()
  let active = 0
  let waiting = 0

  return {
    run(task) {
      waiting += 1
      const next = tail.then(async () => {
        waiting -= 1
        active += 1
        try {
          return await task()
        } finally {
          active -= 1
        }
      })

      // Keep the queue usable when a renderer request fails.
      tail = next.catch(() => {})
      return next
    },

    status() {
      return { active, waiting }
    },
  }
}
