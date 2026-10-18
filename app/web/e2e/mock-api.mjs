import { createServer } from 'node:http'

const host = '127.0.0.1'
const port = Number(process.env.ZHIGU_E2E_MOCK_PORT || 8080)

function json(response, status, body, headers = {}) {
  response.writeHead(status, { 'Content-Type': 'application/json', ...headers })
  response.end(JSON.stringify(body))
}

const server = createServer((request, response) => {
  const path = new URL(request.url || '/', `http://${host}:${port}`).pathname
  if (path === '/healthz') {
    return json(response, 200, { status: 'ok' })
  }
  if (path === '/api/v1/session/csrf') {
    return json(
      response,
      200,
      { data: { csrf_token: 'e2e-csrf-token' }, error: null, trace_id: 'e2e-mock' },
      { 'X-CSRF-Token': 'e2e-csrf-token' },
    )
  }
  return json(response, 404, {
    data: null,
    error: { code: 'E2E_MOCK_NOT_FOUND', message: 'No live API is configured for this browser test.' },
    trace_id: 'e2e-mock',
  })
})

server.listen(port, host)

for (const signal of ['SIGINT', 'SIGTERM']) {
  process.on(signal, () => server.close(() => process.exit(0)))
}
