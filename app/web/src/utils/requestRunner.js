export function createRequestSequence() {
  let current = 0
  return {
    begin() {
      current += 1
      return current
    },
    isCurrent(id) {
      return id === current
    },
    invalidate() {
      current += 1
    },
  }
}

export async function runRequest({ method = 'GET', request, retries = 0, delay = ms => new Promise(resolve => setTimeout(resolve, ms)) }) {
  const attempts = method.toUpperCase() === 'GET' ? Math.max(1, retries + 1) : 1
  let lastError
  for (let attempt = 0; attempt < attempts; attempt += 1) {
    try {
      return await request()
    } catch (error) {
      lastError = error
      if (attempt + 1 < attempts) await delay(250 * (2 ** attempt))
    }
  }
  throw lastError
}
