# Futures worker

Python 3.12, isolated from `app/research-service`. The worker includes a strict FastAPI
execution endpoint, bounded research executor, manifest-only HTTP provider, Go tool/model
gateways and local policy defense. `policy.validate_scope` is not a replacement for
signed grants and owner/run checks at the Go gateway.

Run tests with a Python 3.12 interpreter:

```bash
PYTHONPATH=src python -m unittest discover -s tests -v
```

Dependencies are locked independently in `uv.lock`. Do not install futures packages into
the stock worker. Start only on loopback/intranet and require `FUTURES_SERVICE_TOKEN`.
Full contracts are in `../../contracts/futures/v1`; implementation instructions are in
`../../spec/期货研究模块_开发SPEC.md`.
