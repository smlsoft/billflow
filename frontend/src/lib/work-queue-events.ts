export const WORK_QUEUE_CHANGED_EVENT = 'billflow:work-queue-changed'

export function notifyWorkQueueChanged() {
  if (typeof window === 'undefined') return
  window.dispatchEvent(new Event(WORK_QUEUE_CHANGED_EVENT))
}
