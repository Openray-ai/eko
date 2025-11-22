# 🛡️ Ekō

> Blazing-fast prompt sanitization for AI - Prevent data leaks before they happen

**Ekō** is an open-source security layer that protects your organization from accidentally leaking sensitive data to AI services. Built in Go for maximum performance, it acts as a transparent proxy between your applications and AI providers (OpenAI, Anthropic, Google AI), automatically detecting and sanitizing credentials, PII, and business secrets in real-time.

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

**Python Integration (Anthropic):**
```python
import anthropic

client = anthropic.Anthropic(
    api_key="sk-ant-your-key",
    base_url="http://localhost:8080/v1/anthropic"  # ← Point to Ekō
)

message = client.messages.create(
    model="claude-sonnet-4-20250514",
    messages=[{"role": "user", "content": "My API key is sk-proj-abc123"}]
)
# Ekō automatically sanitizes before sending to Anthropic
```

**Environment Variables (Universal):**
```bash
# Works with any SDK/library
export OPENAI_BASE_URL="http://localhost:8080/v1/openai"
export ANTHROPIC_BASE_URL="http://localhost:8080/v1/anthropic"

# Now all AI calls are automatically protected
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
  "processing_time_ms": 2.1
}
```

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
    
  anthropic:
    enabled: true
    base_url: "https://api.anthropic.com/v1"
    
  behavior:
    on_violation: "sanitize"  # Options: block, sanitize, warn
    log_requests: true
    add_violation_headers: true

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
```bash
# Prometheus metrics
curl http://localhost:8080/metrics

# Health check
curl http://localhost:8080/health
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
- ✅ **Vendor neutral** - Works with OpenAI, Anthropic, Google, local LLMs
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
- Anthropic proxy
- Google AI proxy
- African-specific patterns
- Docker deployment

**🚧 Phase 2: Enterprise (Next)**
- Open WebUI integration
- Admin dashboard
- SSO integration (SAML, LDAP, AD)
- Advanced compliance reporting
- Multi-tenancy support

**📋 Phase 3: Advanced**
- Optional SLM module for contextual detection
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