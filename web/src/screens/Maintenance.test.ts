import { expect, test } from 'vitest'
import { scheduleTime } from './Maintenance'
test('next run uses the schedule zone rather than the browser zone', () => {
 const utc = scheduleTime('2026-07-01T03:00:00Z', 'UTC')
 const madrid = scheduleTime('2026-07-01T03:00:00Z', 'Europe/Madrid')
 expect(utc).toMatch(/3:00|03:00/)
 expect(madrid).toMatch(/5:00|05:00/)
 expect(utc).not.toBe(madrid)
})
