# 🛡️ Ekō

> Blazing-fast prompt sanitization for AI - Prevent data leaks before they happen

**Ekō** is an open-source, context-aware security proxy that prevents your organization from leaking sensitive data to AI services. Built in Go for maximum performance, it sits between your applications and AI providers (OpenAI today; Anthropic and Google on the roadmap) and combines deterministic pattern matching with an optional finetuned SLM for contextual detection — with first-class detectors for African fintech and banking compliance (BVN, NUBAN, NIN, M-Pesa, SA ID, and more).

---

## 🎯 The Problem

Your team wants to use AI for productivity, but every prompt is a potential data breach:
```diff
- User prompt: "Debug this error: postgres://admin:SecureP@ss123@prod-db.company.com"
- User prompt: "Analyze customer feedback from BVN 12345678901 and account 0123456789"
- User prompt: "Here's our API key for testing: sk-proj-abc123xyz..."

+ Ekō sanitized: "Debug this error: postgres://[REDACTED]@[REDACTED]"
+ Ekō sanitized: "Analyze customer feedback from BVN [REDACTED] and account [REDACTED]"
+ Ekō sanitized: "Here's our API key for testing: [REDACTED_API_KEY]"
```

**The risks are real:**
- Production credentials leaked to ChatGPT
- Customer PII sent to third-party AI providers
- Compliance violations (NDPR, POPIA, GDPR, PCI-DSS)
- Business intelligence exposed to competitors
- Regulatory fines and customer trust erosion

**Traditional solutions fail:**
- ❌ Azure OpenAI/AWS Bedrock cost $8K-15K/year and *don't sanitize prompts*
- ❌ Banning AI entirely means losing competitive advantage
- ❌ Security training doesn't prevent human error under pressure
- ❌ Manual review of every prompt is impractical

**Ekō solves this** — automatically, instantly, transparently.

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
│  │    • Redact/Replace       │  │
│  │    • Alert/Log/Block      │  │
│  └───────────┬───────────────┘  │
└──────────────┼───────────────────┘
               │ Sanitized prompt
               ▼
       ┌───────────────┐
       │   AI Provider │
       │ (OpenAI, etc) │
       └───────────────┘
```

---

## 🚀 Quick Start

### Option 1: Proxy Mode (Recommended - Zero Code Changes)

The easiest way to secure your AI stack. Just change your API base URL:

**Docker Deployment:**
```bash
# Run Ekō proxy
docker run -p 8080:8080 openray/eko:latest

# Test it
curl -X POST http://localhost:8080/v1/openai/chat/completions \
  -H "Authorization: Bearer YOUR_OPENAI_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4",
    "messages": [{"role": "user", "content": "My password is admin123"}]
  }'
```

**Python Integration (OpenAI):**
```python
from openai import OpenAI

# Just point to Ekō instead of OpenAI
client = OpenAI(
    api_key="sk-your-key",
    base_url="http://localhost:8080/v1/openai"  # ← Only change needed!
)

# Use normally - sanitization happens automatically
response = client.chat.completions.create(
    model="gpt-4",
    messages=[{
        "role": "user", 
        "content": "Debug: Connection failed to postgres://admin:password@prod-db:5432"
    }]
)

# Check if anything was sanitized
violations = response.response.headers.get('X-Eko-Violations')
if violations and int(violations) > 0:
    print(f"⚠️ Ekō sanitized {violations} sensitive items")
```

**Python Integration (Anthropic) — coming soon:**
```python
# Anthropic proxy support is on the roadmap.
# For now, use the Core API (Option 2 below) to sanitize prompts before
# passing them to the Anthropic SDK directly.
```

**Environment Variables (OpenAI):**
```bash
export OPENAI_BASE_URL="http://localhost:8080/v1/openai"

# Now all OpenAI calls are automatically protected
```

### Option 2: Core API (Custom Integrations)

For custom workflows where you need explicit control:
```bash
# Sanitize a prompt
curl -X POST http://localhost:8080/v1/sanitize \
  -H "Content-Type: application/json" \
  -d '{
    "prompt": "My BVN is 12345678901 and email is john@company.com"
  }'
```

**Response:**
```json
{
  "sanitized_prompt": "My BVN is 00000000001 and email is user_aaa@example.com",
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
  "redacted_count": 0,
  "tokenized_count": 2,
  "session_id": "eko_123e4567-e89b-12d3-a456-426614174000"
}
```

To reuse tokens across requests, pass the same `session_id` back:
```bash
curl -X POST http://localhost:8080/v1/sanitize \
  -H "Content-Type: application/json" \
  -d '{
    "prompt": "My BVN is 12345678901 and email is john@company.com",
    "session_id": "eko_123e4567-e89b-12d3-a456-426614174000"
  }'
```

Note: credentials (e.g. database connection strings, cloud keys) are always redacted, even in tokenization mode.

### Redact vs Tokenize
Ekō supports two sanitization modes:
- `redact`: Replace matched data with labels like `[REDACTED_EMAIL]`. This is the default.
- `tokenize`: Replace matched data with **stable, reversible tokens** within a session (for supported PII/financial patterns). The response includes `session_id` so you can reuse the same tokens in subsequent requests.

Behavior details:
- `/v1/sanitize` returns `session_id` and uses session-aware sanitization. If you don’t provide one, Ekō generates it.
- **Credentials are never tokenized** (API keys, DB connection strings, cloud keys, etc.); they are always redacted.
- `LOG` severity patterns are **not** modified in either mode.

---

## 🌍 What Gets Detected

### 🔐 Credentials & Secrets
- API keys (OpenAI, Anthropic, Google, AWS, Azure)
- Database connection strings (PostgreSQL, MongoDB, MySQL)
- JWT tokens and OAuth credentials
- SSH private keys and certificates
- Environment variables (API_KEY=, PASSWORD=)

### 👤 Personal Identifiable Information (PII)
- **Nigerian**: BVN, NIN, NUBAN account numbers, phone (+234)
- **Kenyan**: M-Pesa formats, ID numbers, phone (+254)
- **South African**: ID numbers (13 digits), phone (+27)
- **Ghanaian**: Mobile Money, phone (+233)
- **Universal**: Emails, generic phone numbers, IBAN, SWIFT codes

### 💳 Financial Data
- Credit card numbers (Luhn algorithm validated)
- Bank account numbers
- CVV codes
- Transaction IDs
- Currency amounts (configurable thresholds)

### 🏢 Custom Business Patterns
- Employee IDs
- Customer account numbers
- Project codenames
- Internal system identifiers
- Any regex pattern you define

---

## 🎨 Configuration

Create a `config.yaml` to customize behavior:
```yaml
server:
  port: 8080
  host: "0.0.0.0"

# Proxy behavior on violations
proxy:
  openai:
    enabled: true
    base_url: "https://api.openai.com/v1"

  # anthropic and google proxies: coming in Phase 2

  behavior:
    on_violation: "sanitize"  # Options: block, sanitize, warn
    log_requests: true
    add_violation_headers: true
    sanitization_mode: "tokenize"  # Options: redact, tokenize
    token_store_backend: "memory"  # Options: memory, redis
    token_ttl_ms: 30000            # Token vault TTL in ms (tokenize mode only)

  redis:
    addr: "127.0.0.1:6379"
    username: ""
    password: ""
    db: 0
    key_prefix: "eko:"
    meta_suffix: "vault_meta:"
    payload_suffix: "vault_payload:"
    pool_size: 10
    min_idle_conns: 2
    dial_timeout_ms: 100
    read_timeout_ms: 100
    write_timeout_ms: 100
    max_retries: 3

  crypto:
    provider: "local"  # Options: local, vault-transit
    active_key_id: "local-dev"
    local_master_key: "BASE64_32_BYTE_AES_KEY"
    fallback_keys: []
    vault_transit:
      address: "https://vault.example.com"
      token: "VAULT_TOKEN"
      namespace: ""
      mount: "transit"
      key_name: "eko-session"
      timeout_ms: 2000
      tls_skip_verify: false
```

> **Secrets**: do not commit `local_master_key` or the Vault `token` to source
> control. Inject them through your secrets manager at deploy time.

#### Rotating the local master key

The Redis-backed store seals each session with a random data key wrapped by
the active master key. To rotate without invalidating live sessions:

1. Add an entry to `crypto.fallback_keys` whose `key_id` equals the current
   `crypto.active_key_id` and whose `master_key` is the current
   `crypto.local_master_key`. Roll this config to **all** replicas first.
2. Set `crypto.active_key_id` and `crypto.local_master_key` to the new pair
   and roll out.
3. New writes use the new key; existing sessions continue to decrypt via the
   fallback entry.
4. Once `proxy.behavior.token_ttl_ms` has elapsed for all live sessions, the
   fallback entry can be removed in a third rollout.

> **Multi-replica caveat**: rolling out steps 1 and 2 in the wrong order — or
> rolling step 2 to some replicas before step 1 reaches all of them — will
> cause replicas on the old config to fail decrypting payloads written by the
> new active key. Failures are non-destructive (ciphertext is preserved) but
> requests will return 503 until config converges.

For Vault Transit, key rotation is performed inside Vault and is transparent
to Eko — the stored key id is `vault-transit:{mount}/{key_name}` and Vault
selects the appropriate key version on decrypt.

```yaml
# Pattern management
patterns:
  config_file: "./patterns/default.yaml"
  custom_patterns_dir: "./patterns/custom"

# Alerting
alerts:
  webhooks:
    - url: "https://hooks.slack.com/services/YOUR/WEBHOOK"
      severity: ["BLOCK"]
  email:
    smtp_host: "smtp.gmail.com"
    smtp_port: 587
    from: "alerts@company.com"
    to: ["security@company.com"]

# Compliance
compliance:
  enable_reporting: true
  report_dir: "./reports"
  retention_days: 90
```

### Add Custom Patterns
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
    
  - name: "customer_account"
    regex: "ACCT[0-9]{10}"
    type: "financial"
    severity: "BLOCK"
    description: "Customer account numbers"
```

### Optional SLM Detection (Contextual)

Regex catches structured tokens (BVN, NUBAN, API keys). For **contextual** PII —
person names, addresses, dates tied to a record — Ekō can call an optional Small
Language Model sidecar that runs `openai/privacy-filter` with the
`iamSamurai/privacy-filter-nigeria` LoRA adapter. When enabled, the SLM runs in
parallel with regex detection and merges into the same redaction/tokenization
pipeline.

The sidecar is shipped in this repo at `slm-sidecar/` and wired up in
`docker-compose.yml` behind a `slm` profile so the default `docker compose up`
stays Go-only. To run with SLM:

```bash
docker compose --profile slm up --build
```

It is **off by default** in Ekō — enable in `configs/config.yaml`:

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
    # Optional per-label overrides (see internal/core/slm/mapping.go):
    # labels:
    #   private_phone: { severity: "BLOCK" }
```

**Failure mode:** soft-fail with circuit breaker. If the sidecar is slow,
unreachable, or returns errors, Ekō logs a warning, increments
`eko_slm_failures_total`, and continues with regex-only detection. Requests
never fail because SLM is down. After `failure_threshold` consecutive failures
the breaker trips and SLM is skipped entirely until `cooldown_ms` has elapsed.

**Per-request opt-in for `POST /v1/sanitize`:** the config flag controls
whether the SLM client is wired up at startup. Independently, the sanitize
endpoint accepts an `slm` boolean in the request body that toggles SLM use
*for that single request*. Defaults to `false` when omitted — callers must
opt in explicitly:

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

Other endpoints (the OpenAI proxy paths) are unaffected — they always use SLM
when the config flag enables it.

See `slm-sidecar/README.md` for sidecar details.

---

## 🏗️ Deployment Options

### Docker (Recommended)
```bash
docker run -d \
  -p 8080:8080 \
  -v $(pwd)/config.yaml:/app/config.yaml \
  -v $(pwd)/patterns:/app/patterns \
  openray/eko:latest
```

### Docker Compose with Open WebUI
```yaml
version: '3.8'
services:
  eko:
    image: openray/eko:latest
    ports:
      - "8080:8080"
    volumes:
      - ./config.yaml:/app/config.yaml
      - ./patterns:/app/patterns
      
  open-webui:
    image: ghcr.io/open-webui/open-webui:main
    ports:
      - "3000:8080"
    environment:
      - OPENAI_API_BASE_URL=http://eko:8080/v1/openai
    depends_on:
      - eko
```

### Kubernetes
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: eko
spec:
  replicas: 3
  selector:
    matchLabels:
      app: eko
  template:
    metadata:
      labels:
        app: eko
    spec:
      containers:
      - name: eko
        image: openray/eko:latest
        ports:
        - containerPort: 8080
        resources:
          requests:
            memory: "128Mi"
            cpu: "100m"
          limits:
            memory: "512Mi"
            cpu: "500m"
```

### Binary (Direct Installation)
```bash
# Download
wget https://github.com/openray/eko/releases/latest/download/eko-linux-amd64

# Make executable
chmod +x eko-linux-amd64

# Run
./eko-linux-amd64 --config config.yaml
```

---

## 📊 Monitoring & Compliance

### Built-in Metrics

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

# HELP eko_errors_total Total number of errors encountered during sanitization
# TYPE eko_errors_total counter
eko_errors_total 2

# HELP eko_uptime_seconds Number of seconds since the process started
# TYPE eko_uptime_seconds gauge
eko_uptime_seconds 3600.00

# HELP eko_goroutines Current number of goroutines
# TYPE eko_goroutines gauge
eko_goroutines 14

# HELP eko_memory_alloc_bytes Currently allocated heap memory in bytes
# TYPE eko_memory_alloc_bytes gauge
eko_memory_alloc_bytes 4823040

# HELP eko_memory_sys_bytes Total memory obtained from the OS in bytes
# TYPE eko_memory_sys_bytes gauge
eko_memory_sys_bytes 24379392
```

These metrics can be scraped directly by Prometheus. Add Ekō to your `prometheus.yml`:
```yaml
scrape_configs:
  - job_name: 'eko'
    static_configs:
      - targets: ['localhost:8080']
    metrics_path: /metrics
```

```bash
# Health check
curl http://localhost:8080/health
curl http://localhost:8080/ready
```

### Violation Logging
Every sanitization is logged for compliance:
```json
{
  "timestamp": "2025-01-15T10:30:45Z",
  "user_id": "user@company.com",
  "violation_type": "api_key",
  "pattern": "openai_api_key",
  "severity": "BLOCK",
  "provider": "openai",
  "action_taken": "sanitized",
  "compliance_frameworks": ["NDPR", "PCI-DSS"]
}
```

### Compliance Reports
Generate audit-ready reports:
```bash
# Export violations for last 30 days
curl http://localhost:8080/v1/compliance/report?days=30 > compliance-report.pdf

# CSV export for analysis
curl http://localhost:8080/v1/compliance/export?format=csv > violations.csv
```

---

## 🚦 Performance

Ekō is built for production scale:

| Metric | Target | Actual |
|--------|--------|--------|
| **Core API Latency (p95)** | <5ms | 2-4ms |
| **Proxy End-to-End (p95)** | <50ms | 25-45ms |
| **Throughput** | 1000 req/s | 1200+ req/s |
| **Memory Footprint** | <100MB | ~50MB |
| **Concurrent Connections** | 500+ | Tested to 1000+ |
| **Pattern Accuracy** | >95% | 97.3% |
| **False Positive Rate** | <2% | 1.4% |

*Benchmarked on: 4 vCPU, 8GB RAM, 1000 concurrent requests*

---

## 🤝 Why Ekō?

**vs Azure OpenAI / AWS Bedrock:**
- ✅ **60-80% cheaper** - $3K-10K/year vs $8K-15K/year
- ✅ **Actually sanitizes prompts** - Azure/AWS don't inspect content
- ✅ **Vendor neutral** - OpenAI proxy today; Anthropic and Google on the roadmap. The core `/v1/sanitize` API works with any LLM you call yourself.
- ✅ **African compliance** - Built-in NDPR, POPIA patterns
- ✅ **Use both** - Deploy Ekō in front of Azure OpenAI for defense-in-depth

**vs Manual Security Training:**
- ✅ **Automatic** - No reliance on human memory
- ✅ **Consistent** - Works 24/7, never gets tired
- ✅ **Audit trail** - Proof of protection for regulators

**vs DIY Solutions:**
- ✅ **Production-ready** - Battle-tested patterns
- ✅ **Maintained** - Regular updates for new threats
- ✅ **Support available** - Enterprise SLAs

**vs Other AI Gateways (Portkey, LiteLLM):**
- ✅ **Security-first** - Deep sanitization vs basic filtering
- ✅ **Compliance-ready** - Built for regulatory requirements
- ✅ **African expertise** - Understands local data formats

---

## 🌟 Use Cases

### Financial Services
Deploy Ekō as infrastructure to protect customer data while enabling AI productivity:
```
500-person bank → Open WebUI → Ekō → Claude API
                               ↓
                    Blocks 200+ violations/month
                    (BVNs, account numbers, credentials)
```

### Healthcare
HIPAA/local health data protection:
```
Patient: "Analyze symptoms for patient ID MRN-123456"
         ↓
Ekō:     "Analyze symptoms for patient ID [REDACTED_MRN]"
```

### Software Development
Prevent credential leaks during debugging:
```
Developer: "Fix error: FATAL: password authentication failed for user 'admin'"
           ↓
Ekō:       "Fix error: FATAL: password authentication failed for user '[REDACTED]'"
```

### Enterprise Knowledge Work
Safe AI adoption across departments:
```
Marketing → Ekō → OpenAI (sanitized campaigns)
Finance   → Ekō → Claude (sanitized analysis)
Legal     → Ekō → Local LLM (stays on-prem)
```

---

## 🗺️ Roadmap

**✅ Phase 1: Core (Current)**
- Core detection engine
- OpenAI proxy
- African-specific patterns (BVN, M-Pesa, NUBAN, regional phones)
- Prometheus-compatible metrics endpoint
- Docker deployment

**🚧 Phase 2: Enterprise (Next)**
- Anthropic proxy
- Google AI proxy
- Open WebUI integration
- Admin dashboard
- SSO integration (SAML, LDAP, AD)
- Advanced compliance reporting
- Multi-tenancy support

**🚧 Phase 3: Advanced (in progress)**
- Optional SLM module for contextual detection (`slm-sidecar/`, opt-in via `proxy.slm.enabled`)
- Smart routing (local vs API based on sensitivity)
- Cost analytics per provider
- GraphQL/gRPC proxy support
- Custom model fine-tuning

---

## 🤝 Contributing

We welcome contributions! Areas where we especially need help:

- **New patterns** for African contexts (formats, regulations, institutions)
- **Provider integrations** (Cohere, Mistral, etc.)
- **Documentation** (tutorials, use cases, translations)
- **Testing** (edge cases, performance benchmarks)

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

---

## 📜 License

MIT License - see [LICENSE](LICENSE) for details.

---

## 🌍 About OpenRay

[OpenRay](https://openray.ai) is an open-source AI foundation driving local innovation in Africa. We build tools that make AI safer, more accessible, and contextually relevant for African developers and businesses.

**Other Projects:**
- Coming soon...

---

## 💬 Community & Support

- **GitHub Discussions**: [Ask questions, share ideas](https://github.com/openray/eko/discussions)
- **Twitter**: [@OpenRayAI](https://twitter.com/OpenRayAI)
- **Email**: hello@openray.ai
- **Enterprise Support**: enterprise@openray.ai

---

## ⚡ Quick Links

- [🐛 Report a Bug](https://github.com/openray/eko/issues/new?template=bug_report.md)
- [💡 Request a Feature](https://github.com/openray/eko/issues/new?template=feature_request.md)
- [🎯 Suggest a Pattern](https://github.com/openray/eko/issues/new?template=pattern_suggestion.md)
- [📖 Read the Docs](https://docs.openray.ai/eko)

---

<p align="center">
  <strong>Built with ❤️ in Africa, for the world</strong>
</p>

<p align="center">
  <sub>Ekō (Yoruba): "to guard, to protect" — Because your data deserves protection</sub>
</p>
