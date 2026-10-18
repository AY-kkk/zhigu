import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import * as api from '../../src/modules/futures/api.js'
import { useFuturesStore } from '../../src/modules/futures/store.js'

vi.mock('../../src/modules/futures/api.js', () => ({
  createRun: vi.fn(),
  batchReadNotifications: vi.fn(),
  getNotifications: vi.fn(),
  getCapabilities: vi.fn(),
  getRun: vi.fn(),
}))

describe('futures store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
  })

  it('reuses an idempotency key until a submission settles', async () => {
    api.createRun.mockResolvedValue({ id: 'run-1' })
    const store = useFuturesStore()
    const first = store.mutationKey('research.create')
    const second = store.mutationKey('research.create')
    expect(first).toBe(second)
    await store.startResearch({ draft_id: 'd1' })
    expect(api.createRun).toHaveBeenCalledWith({ draft_id: 'd1' }, first)
    expect(store.mutationKeys['research.create']).toBeUndefined()
  })

  it('rolls back every optimistic batch-read row on failure', async () => {
    api.batchReadNotifications.mockRejectedValue(new Error('failed'))
    const store = useFuturesStore()
    store.notifications = [
      { id: 'a', read_at: null },
      { id: 'b', read_at: '2026-01-01T00:00:00Z' },
    ]
    await expect(store.batchReadNotifications(['a', 'b'])).rejects.toThrow('failed')
    expect(store.notifications[0].read_at).toBeNull()
    expect(store.notifications[1].read_at).toBe('2026-01-01T00:00:00Z')
  })

  it('invalidates stale account state and stops old polling', () => {
    const store = useFuturesStore()
    store.resources.runs = [{ id: 'old' }]
    store.pollers['run:old'] = setTimeout(() => {}, 1000)
    store.resetForAccount('next')
    expect(store.generation).toBe(1)
    expect(store.resources.runs).toBeUndefined()
    expect(store.pollers['run:old']).toBeUndefined()
  })
})
