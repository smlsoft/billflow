export const WORK_QUEUE_CHANGED_EVENT = 'billflow:work-queue-changed'

export function notifyWorkQueueChanged() {
  window.dispatchEvent(new CustomEvent(WORK_QUEUE_CHANGED_EVENT))
}
