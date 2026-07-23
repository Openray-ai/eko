# 🛡️ Ekō

**Ekō** is an open-source, context-aware security proxy that prevents your organization from leaking sensitive data to AI services. It sits between your application and an AI provider, detects sensitive data in prompts, and either redacts or tokenizes that data before it leaves your environment — with first-class detectors for African regulated domains.

<p align="center">
  <a href="https://ek-playground.openray.workers.dev/"><strong>🎮 Try the live playground →</strong></a>
</p>

> Paste a prompt, pick redact or anonymize (enable **Advanced** for contextual (SLM) detection), and watch Ekō detect and sanitize it in real time.

---

## 🎯 The Problem

Your team wants to use AI for productivity, but every prompt is a potential data breach:
```diff
- User prompt: "Amina Yusuf can be reached at +234 802 111 3344. Her BVN is 22334455667 and the team debugs against postgres://admin:p4ss@prod-db:5432/core."

+ Ekō sanitized: "Amina Yusuf can be reached at [REDACTED_PHONE]. Her BVN is [REDACTED_BVN] and the team debugs against [REDACTED_DB_CONNECTION]."
```

Regex detection catches structured identifiers (phone, BVN, DB URLs) on its own. Contextual PII such as the person's name and private address are caught when the optional [SLM detector](#-optional-slm-detection-contextual) is enabled.

---

## ✨ How It Works

Ekō sits between your applications and AI providers, inspecting and sanitizing every prompt:
```
┌─────────────┐
│  Your App   │
│  (ChatGPT   │
│   Clone)    │
└──────┬──────┘
       │
       │ Original prompt
       ▼
┌─────────────────────────────────┐
│         Ekō Proxy               │
│  ┌───────────────────────────┐  │
│  │  Pattern Detection Engine │  │
│  │  • API Keys               │  │
│  │  • Credentials            │  │
│  │  • PII (BVN, emails, etc) │  │
│  │  • Financial Data         │  │
│  │  • Custom Patterns        │  │
│  └───────────┬───────────────┘  │
│              │                   │
│  ┌───────────▼───────────────┐  │
│  │    Sanitization Logic     │  │
│  │    • Redact / Tokenize    │  │
│  │    • Alert / Log / Block  │  │
│  └───────────┬───────────────┘  │
└──────────────┼───────────────────┘
               │ Sanitized prompt
               ▼
       ┌───────────────┐
       │   AI Provider │
       │ (OpenAI API)  │
       └───────────────┘
```

Use Ekō when teams need to adopt AI tooling without sending raw credentials, customer records, regulated identifiers, or internal business data directly to third-party model providers.

---

## 📋 Current Status

| Area | Status |
| --- | --- |
| Core sanitization API (`POST /v1/sanitize`) | ✅ Available |
| Batch sanitization API (`POST /v1/sanitize/batch`) | ✅ Available |
| Multi-provider chat completions proxy (`POST /v1/chat/completions`) | ✅ Available |
| Multi-provider Responses proxy (`POST /v1/responses`) | ✅ Available |
| Prometheus metrics | ✅ Available |
| Redaction mode | ✅ Available |
| Tokenization mode | ✅ Available |
| Redis-backed token vault | ✅ Available |
| Optional SLM sidecar | ✅ Available, opt-in |
| Model routing for OpenAI, Anthropic, Gemini, and DeepSeek | ✅ Available |
| Compliance report export endpoints | 🚧 Roadmap |

---

## 🚀 Quick Start

### Option 1: Run Locally With Go

Requirements: **Go 1.24+**

```bash
make install
make run-config
```

The server starts on `http://localhost:8080` using `configs/config.example.yaml`. Verify it is up:

```bash
curl http://localhost:8080/health
curl http://localhost:8080/ready
```

### Option 2: Run With Docker Compose

`docker-compose.yml` expects a local config file:

```bash
cp configs/config.example.yaml configs/config.yaml
mkdir -p reports patterns/custom
docker compose up --build
```

### Option 3: Run the Docker Image Directly

```bash
docker run --rm -p 8080:8080 openray/eko:main-latest
```

If you need custom configuration or patterns, prefer Docker Compose so the required files can be mounted explicitly.

---

## 🔌 Multi-Provider Proxy (Recommended — Near-Zero Code Changes)

The easiest way to secure your AI stack. Ekō exposes OpenAI-compatible routes under `/v1` and routes internally by the requested `model`:

- `POST /v1/chat/completions`
- `POST /v1/responses`

Just point the OpenAI SDK at Ekō instead of OpenAI:

```python
from openai import OpenAI

client = OpenAI(
    api_key="sk-your-key",
    base_url="http://localhost:8080/v1",  # ← Only change needed!
)

response = client.chat.completions.create(
    model="gpt-4",
    messages=[{
        "role": "user",
        "content": "Debug this: postgres://admin:password@prod-db:5432/app",
    }],
)
```

**Environment variable form:**
```bash
export OPENAI_BASE_URL="http://localhost:8080/v1"
# Now supported OpenAI-compatible calls are automatically protected
```

Ekō forwards the sanitized request to the configured upstream provider selected by model routing. For enabled providers, default prefixes route `gpt-*` and `o*` models to OpenAI, `claude-*` to Anthropic, `gemini-*` to Gemini, and `deepseek-*` to DeepSeek. For non-streaming requests in tokenization mode, Ekō can also resolve response tokens back through the session vault before returning the response. Streaming requests are sanitized in redaction mode because response token resolution is not supported on streamed chunks.

**Model routing examples:**

```bash
# OpenAI
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer $OPENAI_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}]}'

# Anthropic through the same public route
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "x-api-key: $ANTHROPIC_API_KEY" \
  -H "anthropic-version: 2023-06-01" \
  -H "Content-Type: application/json" \
  -d '{"model":"claude-3-5-sonnet-latest","messages":[{"role":"user","content":"hello"}]}'

# Gemini through the same public route
curl -X POST "http://localhost:8080/v1/chat/completions?key=$GEMINI_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"gemini-1.5-pro","messages":[{"role":"user","content":"hello"}]}'

# DeepSeek through the same public route
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer $DEEPSEEK_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"deepseek-chat","messages":[{"role":"user","content":"hello"}]}'
```

Provider adapters fail closed: unsupported tools, multimodal payloads, JSON mode, or provider-incompatible Responses features return `400` instead of being silently dropped. `/v1/responses` remains one endpoint and uses provider-specific internal response managers for compatible text workflows.

**Useful response headers:**

| Header | Meaning |
| --- | --- |
| `X-Eko-Session-ID` | Session used for tokenization and response resolution |
| `X-Eko-Violations-Found` | Number of detected violations |
| `X-Eko-Redacted-Count` | Number of redacted spans |
| `X-Eko-Tokens-Issued` | Number of tokens created |
| `X-Eko-Sanitization-Mode` | Active mode, usually `redact` or `tokenize` |
| `X-Eko-Sanitization-Override` | Explains forced behavior, such as streaming fallback |
| `X-Eko-Resolve-Status` | Response token resolution result |
| `X-Eko-Violation-Summary` | Compact summary of violation types |

---

## 🧩 Core API — `POST /v1/sanitize`

For custom workflows where you need explicit control over sanitization before calling an AI provider yourself:

```bash
curl -X POST http://localhost:8080/v1/sanitize \
  -H "Content-Type: application/json" \
  -d '{
    "prompt": "My BVN is 12345678901 and email is john@company.com"
  }'
```

**Request fields:**

| Field | Required | Description |
| --- | --- | --- |
| `prompt` | Yes | Text to inspect and sanitize |
| `session_id` | No | Existing Ekō session for stable token reuse |
| `sanitization_mode` | No | Per-request override: `redact` or `tokenize` |
| `slm` | No | Opt this request into contextual SLM detection (when enabled) |

**Response:**
```json
{
  "original_prompt": "My BVN is 12345678901 and email is john@company.com",
  "sanitized_prompt": "My BVN is [REDACTED_BVN] and email is [REDACTED_EMAIL]",
  "violations": [
    {
      "type": "pii",
      "severity": "BLOCK",
      "pattern": "nigerian_bvn",
      "matched": "12345678901",
      "position": 10
    },
    {
      "type": "pii",
      "severity": "BLOCK",
      "pattern": "email",
      "matched": "john@company.com",
      "position": 35
    }
  ],
  "safe": false,
  "processing_time_ms": 2.1,
  "redacted_count": 2,
  "tokenized_count": 0,
  "session_id": "eko_123e4567-e89b-12d3-a456-426614174000"
}
```

The exact redaction labels depend on the matched pattern definitions.

To reuse tokens across requests, pass the same `session_id` back:
```bash
curl -X POST http://localhost:8080/v1/sanitize \
  -H "Content-Type: application/json" \
  -d '{
    "prompt": "My BVN is 12345678901 and email is john@company.com",
    "session_id": "eko_123e4567-e89b-12d3-a456-426614174000"
  }'
```

If you don't provide a `session_id`, Ekō generates one and returns it.

---

## 🧩 Batch API — `POST /v1/sanitize/batch`

For bulk workflows, submit multiple sanitization items in one request:

```bash
curl -X POST http://localhost:8080/v1/sanitize/batch \
  -H "Content-Type: application/json" \
  -d '{
    "items": [
      {
        "id": "prompt-001",
        "prompt": "My BVN is 12345678901",
        "sanitization_mode": "redact"
      },
      {
        "id": "prompt-002",
        "prompt": "Email jane@example.com before sending this to the model",
        "sanitization_mode": "mask",
        "slm": false
      }
    ]
  }'
```

The top-level request accepts only `items`. Each item has the same controls as the single sanitize endpoint:

| Field | Required | Description |
| --- | --- | --- |
| `id` | No | Caller-provided identifier returned in the matching result |
| `prompt` | Yes | Text to inspect and sanitize |
| `session_id` | No | Existing Ekō session for stable token reuse |
| `sanitization_mode` | No | Per-item override: `redact` or `tokenize` |
| `slm` | No | Opt this item into contextual SLM detection |

Batch responses use partial success. Valid items are processed even when another item fails validation:

```json
{
  "results": [
    {
      "id": "prompt-001",
      "ok": true,
      "result": {
        "sanitized_prompt": "My BVN is [REDACTED_BVN]",
        "violations": [
          {
            "type": "pii",
            "severity": "BLOCK",
            "pattern": "nigerian_bvn",
            "matched": "12345678901",
            "position": 10
          }
        ],
        "safe": false,
        "processing_time_ms": 1.2,
        "redacted_count": 1,
        "tokenized_count": 0,
        "session_id": "eko_123e4567-e89b-12d3-a456-426614174000"
      }
    },
    {
      "id": "prompt-002",
      "ok": false,
      "error": {
        "code": "invalid_sanitization_mode",
        "message": "invalid sanitization_mode"
      }
    }
  ],
  "summary": {
    "total": 2,
    "succeeded": 1,
    "failed": 1,
    "violations": 1,
    "redacted": 1,
    "tokenized": 0,
    "processing_time_ms": 2.4
  }
}
```

Batch safety limits are config-backed:

```yaml
proxy:
  behavior:
    max_batch_items: 100
    max_prompt_bytes: 65536
    max_batch_bytes: 1048576
    max_batch_concurrency: 1
```

---

## 🔁 Redact vs Tokenize

Ekō supports two sanitization modes:

- **`redact`** — replace matched data with labels like `[REDACTED_EMAIL]`. This is the default.
- **`tokenize`** — replace matched data with **stable, reversible tokens** within a session (for supported PII/financial patterns). Reuse the same `session_id` to keep replacements consistent across requests.

Behavior details:
- **Credentials are never tokenized** (API keys, DB connection strings, cloud keys, etc.) — they are always redacted, even in tokenization mode.
- `LOG` severity patterns are **not** modified in either mode.

---

## 🌍 What Gets Detected

### 🔐 Credentials & Secrets
- API keys (OpenAI, Anthropic, Google, AWS, Azure)
- Database connection strings (PostgreSQL, MongoDB, MySQL)
- JWT tokens and OAuth credentials
- SSH private keys and certificates
- Environment-style secrets (`API_KEY=`, `PASSWORD=`)

### 👤 Personal Identifiable Information (PII)
- **Nigerian**: BVN, NIN, NUBAN account numbers, phone (+234)
- **Kenyan**: M-Pesa formats, ID numbers, phone (+254)
- **South African**: ID numbers (13 digits), phone (+27)
- **Ghanaian**: Mobile Money, phone (+233)
- **Universal**: emails, generic phone numbers, IBAN, SWIFT/BIC codes

### 💳 Financial Data
- Credit card numbers
- Bank account numbers
- Transaction references

### 🏢 Custom Business Patterns
Add organization-specific patterns under `patterns/custom`:
```yaml
# patterns/custom/company-secrets.yaml
patterns:
  - name: "employee_id"
    regex: "EMP-[0-9]{6}"
    type: "pii"
    severity: "BLOCK"
    description: "Company employee ID format"

  - name: "project_codename"
    regex: "Project\\s+(Phoenix|Atlas|Titan)"
    type: "business_intelligence"
    severity: "WARN"
    description: "Internal project codenames"
```

---

## 🎨 Configuration

Start from the example config:
```bash
cp configs/config.example.yaml configs/config.yaml
```

Important sections:

| Section | Purpose |
| --- | --- |
| `server` | Host and port |
| `logging` | Log level, format, color, and output file |
| `proxy.openai` | OpenAI proxy enablement, upstream base URL, and timeout |
| `proxy.anthropic` | Anthropic provider enablement, upstream base URL, and timeout |
| `proxy.gemini` | Gemini provider enablement, upstream base URL, and timeout |
| `proxy.deepseek` | DeepSeek provider enablement, upstream base URL, and timeout |
| `proxy.model_routing` | Exact model and prefix routing to upstream providers |
| `proxy.behavior` | Violation behavior, sanitization mode, token store, and TTL |
| `proxy.redis` | Redis-backed token vault settings |
| `proxy.crypto` | Local or Vault Transit key provider settings |
| `proxy.slm` | Optional contextual SLM detector settings |
| `patterns` | Default and custom pattern locations |
| `alerts` | Webhook and email alerting |
| `compliance` | Reporting, retention, and allowed browser origins |

> **Secrets**: do not commit `local_master_key`, the Vault `token`, or provider API keys to source control. Inject them through your secrets manager at deploy time.

### Alerting & Compliance

```yaml
alerts:
  webhooks:
    - url: "https://hooks.slack.com/services/YOUR/WEBHOOK"
      severity: ["BLOCK"]
  email:
    smtp_host: "smtp.gmail.com"
    smtp_port: 587
    from: "alerts@company.com"
    to: ["security@company.com"]

compliance:
  enable_reporting: true
  report_dir: "./reports"
  retention_days: 90
  allowed_origins: ["http://localhost:3000", "https://eko.openray.ai"]
```

### 🔐 Rotating the local master key

The Redis-backed store seals each session with a random data key wrapped by the active master key. To rotate without invalidating live sessions:

1. Add an entry to `crypto.fallback_keys` whose `key_id` equals the current `crypto.active_key_id` and whose `master_key` is the current `crypto.local_master_key`. Roll this config to **all** replicas first.
2. Set `crypto.active_key_id` and `crypto.local_master_key` to the new pair and roll out.
3. New writes use the new key; existing sessions continue to decrypt via the fallback entry.
4. Once `proxy.behavior.token_ttl_ms` has elapsed for all live sessions, the fallback entry can be removed in a third rollout.

> **Multi-replica caveat**: rolling out steps 1 and 2 in the wrong order — or rolling step 2 to some replicas before step 1 reaches all of them — will cause replicas on the old config to fail decrypting payloads written by the new active key. Failures are non-destructive (ciphertext is preserved) but requests will return 503 until config converges.

For Vault Transit, key rotation is performed inside Vault and is transparent to Ekō — the stored key id is `vault-transit:{mount}/{key_name}` and Vault selects the appropriate key version on decrypt.

---

## 🧠 Optional SLM Detection (Contextual)

Regex catches structured tokens (BVN, NUBAN, API keys). For **contextual** PII — person names, addresses, dates tied to a record — Ekō can call an optional Small Language Model sidecar that runs `openai/privacy-filter` with the `iamSamurai/privacy-filter-nigeria` LoRA adapter. When enabled, the SLM runs in parallel with regex detection and merges into the same redaction/tokenization pipeline.

The sidecar ships in this repo at `slm-sidecar/` and is wired into `docker-compose.yml` behind a `slm` profile so the default `docker compose up` stays Go-only:

```bash
docker compose --profile slm up --build
```

It is **off by default** — enable it in `configs/config.yaml`:

```yaml
proxy:
  slm:
    enabled: true
    endpoint: "http://slm-sidecar:8000"
    timeout_ms: 800
    max_input_bytes: 16384       # skip SLM for inputs larger than this
    breaker:
      failure_threshold: 5       # consecutive failures before tripping
      cooldown_ms: 30000         # how long to skip SLM after tripping
```

**Failure mode:** soft-fail with circuit breaker. If the sidecar is slow, unreachable, or returns errors, Ekō logs a warning, increments `eko_slm_failures_total`, and continues with regex-only detection. Requests never fail because SLM is down. After `failure_threshold` consecutive failures the breaker trips and SLM is skipped entirely until `cooldown_ms` has elapsed.

**Per-request opt-in for `POST /v1/sanitize`:** the config flag controls whether the SLM client is wired up at startup. Independently, the sanitize endpoint accepts an `slm` boolean in the request body that toggles SLM use *for that single request*. It defaults to `false` when omitted — callers must opt in explicitly:

```bash
# Regex-only (default behaviour)
curl -X POST localhost:8080/v1/sanitize \
  -H 'Content-Type: application/json' \
  -d '{"prompt":"Amina Yusuf called from +234 802 111 3344"}'

# Opt this request in to SLM
curl -X POST localhost:8080/v1/sanitize \
  -H 'Content-Type: application/json' \
  -d '{"prompt":"Amina Yusuf called from +234 802 111 3344","slm":true}'
```

The OpenAI proxy paths always use SLM when the config flag enables it. See `slm-sidecar/README.md` for sidecar details.

---

## 🏗️ Deployment

```bash
docker run -d \
  -p 8080:8080 \
  -v $(pwd)/configs/config.yaml:/app/configs/config.yaml:ro \
  -v $(pwd)/patterns:/app/patterns \
  openray/eko:main-latest
```

---

## 📊 Monitoring

Health and readiness:
```bash
curl http://localhost:8080/health
curl http://localhost:8080/ready
```

Ekō exposes a Prometheus-compatible plaintext metrics endpoint:
```bash
curl http://localhost:8080/metrics
```

Example output:
```
# HELP eko_requests_total Total number of HTTP requests received
# TYPE eko_requests_total counter
eko_requests_total 1024

# HELP eko_sanitizations_total Total number of sanitization operations performed
# TYPE eko_sanitizations_total counter
eko_sanitizations_total 987

# HELP eko_violations_total Total number of violations detected across all sanitizations
# TYPE eko_violations_total counter
eko_violations_total 312

# HELP eko_uptime_seconds Number of seconds since the process started
# TYPE eko_uptime_seconds gauge
eko_uptime_seconds 3600.00

# HELP eko_batch_requests_total Total number of batch sanitization requests received
# TYPE eko_batch_requests_total counter
eko_batch_requests_total 12
```

Add Ekō to your `prometheus.yml`:
```yaml
scrape_configs:
  - job_name: "eko"
    static_configs:
      - targets: ["localhost:8080"]
    metrics_path: /metrics
```

See `docs/BENCHMARKS.md` for component-level benchmark methodology and baseline results.

---

## 🌟 Use Cases

### Financial Services
Deploy Ekō as infrastructure to protect customer data while enabling AI productivity:
```
Bank → Open WebUI → Ekō → OpenAI API
                     ↓
        Blocks BVNs, account numbers, and credentials before they leave
```

### Healthcare
Health data protection:
```
Patient: "Analyze symptoms for patient ID MRN-123456"
         ↓
Ekō:     "Analyze symptoms for patient ID [REDACTED]"
```

### Software Development
Prevent credential leaks during debugging:
```
Developer: "Fix error: connection failed to postgres://admin:password@prod-db:5432"
           ↓
Ekō:       "Fix error: connection failed to [REDACTED_DB_CONNECTION]"
```

### Enterprise Knowledge Work
Safe AI adoption across departments — Marketing, Finance, and Legal all route through Ekō before reaching the model provider.

---

## 🗺️ Roadmap

**✅ Phase 1: Core (Current)**
- Core detection engine
- OpenAI-compatible chat completions + Responses proxy
- Model routing for OpenAI, Anthropic, Gemini, and DeepSeek
- African-specific patterns (BVN, M-Pesa, NUBAN, regional phones)
- Redaction and session-aware tokenization (memory + Redis vault)
- Prometheus-compatible metrics endpoint
- Docker / Docker Compose deployment

**🚧 Phase 2: Enterprise (Next)**
- Expanded provider capabilities for tools, multimodal inputs, and richer streaming normalization
- Open WebUI deployment examples
- Admin dashboard
- SSO integration (SAML, LDAP, AD)
- Advanced compliance reporting & export endpoints
- Multi-tenant policy management

**🚧 Phase 3: Advanced (in progress)**
- Optional SLM module for contextual detection (`slm-sidecar/`, opt-in via `proxy.slm.enabled`)
- Smart routing (local vs API based on sensitivity)
- Cost analytics per provider
- More regional detection patterns

---

## 🤝 Contributing

We welcome contributions! Areas where we especially need help:

- **New patterns** for African contexts (formats, regulations, institutions)
- **Provider integrations** (Cohere, Mistral, additional OpenAI-compatible providers, etc.)
- **Documentation** (tutorials, use cases, translations)
- **Testing** (edge cases, performance benchmarks)

See [CONTRIBUTING.md](CONTRIBUTING.md) for the workflow, [SECURITY.md](SECURITY.md) for vulnerability reporting, and [PROJECT_STRUCTURE.md](PROJECT_STRUCTURE.md) for package layout.

---

## 📜 License

MIT License — see [LICENSE](LICENSE) for details.

---

## 🌍 About OpenRay

[OpenRay](https://openray.ai) is an open-source AI foundation driving local innovation in Africa. We build tools that make AI safer, more accessible, and contextually relevant for African developers and businesses.

---

## 💬 Community & Support

- **GitHub Discussions**: [Ask questions, share ideas](https://github.com/Openray-ai/eko/discussions)
- **Twitter**: [@OpenRayAI](https://twitter.com/OpenRayAI)

## ⚡ Quick Links

- [🎮 Live Playground](https://ek-playground.openray.workers.dev/)
- [🐛 Report a Bug](https://github.com/Openray-ai/eko/issues/new?template=bug_report.md)
- [💡 Request a Feature](https://github.com/Openray-ai/eko/issues/new?template=feature_request.md)
- [🎯 Suggest a Pattern](https://github.com/Openray-ai/eko/issues/new?template=pattern_suggestion.md)

---

<p align="center">
  <strong>Built with ❤️ in Africa, for the world</strong>
</p>

<p align="center">
  <sub>Ekō (Yoruba): "to guard, to protect" — Because your data deserves protection</sub>
</p>
