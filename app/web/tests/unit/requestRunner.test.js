import { describe, expect, it, vi } from 'vitest'
import { createRequestSequence, runRequest } from '../../src/utils/requestRunner'

describe('requestRunner', () => {
  it('never retries side effects and bounds GET retries', async () => {
    const get = vi.fn().mockRejectedValue(new Error('network'))
    await expect(runRequest({ method: 'GET', request: get, retries: 2, delay: () => 0 })).rejects.toThrow('network')
    expect(get).toHaveBeenCalledTimes(3)
    const post = vi.fn().mockRejectedValue(new Error('network'))
    await expect(runRequest({ method: 'POST', request: post, retries: 5, delay: () => 0 })).rejects.toThrow('network')
    expect(post).toHaveBeenCalledTimes(1)
  })

  it('discards stale sequence results', () => {
    const sequence = createRequestSequence()
    const first = sequence.begin()
    const second = sequence.begin()
    expect(sequence.isCurrent(first)).toBe(false)
    expect(sequence.isCurrent(second)).toBe(true)
  })
})
