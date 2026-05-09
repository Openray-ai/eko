# Ekō SLM Sidecar

Contextual PII detection sidecar for [Ekō](https://github.com/Openray-ai/eko). Loads
[`openai/privacy-filter`](https://huggingface.co/openai/privacy-filter) plus the
[`iamSamurai/privacy-filter-nigeria`](https://huggingface.co/iamSamurai/privacy-filter-nigeria)
LoRA adapter and exposes a small FastAPI for token-classification span detection.

This service is **optional**. Ekō works without it; enable via `proxy.slm.enabled: true`
in `configs/config.yaml` and point `proxy.slm.endpoint` at this container.

## Endpoints

- `GET /healthz` — `200 {"status":"ok","model_loaded":true}` once warmup finishes; `503 {"status":"warming"}` during startup.
- `POST /predict` — body `{"text": "..."}` → `{"spans": [{label, text, start, end, score}, ...]}`
- `POST /predict_batch` — body `{"texts": ["...", "..."]}` → `{"results": [{spans: [...]}, ...]}` (positional)

`start`/`end` are **UTF-8 byte offsets**, matching Ekō's `Violation.Position/End`.

## Configuration

| Env var | Default | Notes |
| --- | --- | --- |
| `PRIVACY_FILTER_MODEL_NAME` | `openai/privacy-filter` | Base model (HF repo id or local path) |
| `PRIVACY_FILTER_ADAPTER_NAME` | `iamSamurai/privacy-filter-nigeria` | Optional LoRA adapter; unset to run base only |
| `PRIVACY_FILTER_DEVICE` | `cpu` | `cuda` if available |
| `PRIVACY_FILTER_PORT` | `8000` | (used by the container CMD only) |

## Local development

```bash
uv sync
uv run uvicorn app.main:app --port 8000 --reload
```

First boot pulls the model + adapter from Hugging Face (~hundreds of MB) and runs a
warmup inference; expect 60–120 s on CPU before `/healthz` returns 200.

```bash
curl -i localhost:8000/healthz
curl -X POST localhost:8000/predict \
  -H 'Content-Type: application/json' \
  -d '{"text":"Amina Yusuf can be reached at +234 802 111 3344."}'
```

## Notes

- The `openai/privacy-filter` adapter from `iamSamurai/privacy-filter-nigeria` is a
  research preview that is recall-oriented; downstream callers should expect to tune
  thresholds, add deterministic filters, or finetune for precision-sensitive use cases.
  See the [model card](https://huggingface.co/iamSamurai/privacy-filter-nigeria) before production use.
- Confidence scores in the `score` field are model outputs, not privacy or compliance guarantees.

## Security

This service has **no authentication and no authorization**. It accepts arbitrary
text and runs inference. **Never expose it publicly.** It is designed to run on
an internal-only network (the `eko-network` bridge in `docker-compose.yml`) with
Ekō as the only client. If you deploy outside compose, ensure the listener is
bound to a private interface and protected at the network layer.

The service also logs request inputs at debug level only. Do not enable debug
logging in production unless your observability stack treats those logs as
sensitive.
