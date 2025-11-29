# OpenRay Guardrail - Implementation Plan (Updated with Proxy API)

## Phase 1: Core Infrastructure (Week 1-2)

### Epic 1.1: Project Setup
- [ ] Initialize Go project with proper module structure
- [ ] Set up CI/CD pipeline (GitHub Actions)
- [ ] Configure Docker build pipeline
- [ ] Create initial README with project vision and quick start
- [ ] Set up test framework and coverage reporting

### Epic 1.2: Core Detection Engine
- [ ] Build regex pattern manager (load, compile, cache patterns)
- [ ] Implement pattern matching engine with concurrent processing
- [ ] Create sanitization logic (detect + redact/replace)
- [ ] Build response formatter (original vs sanitized diff)
- [ ] Write comprehensive unit tests for detection engine

### Epic 1.3: African-Specific Pattern Library
- [ ] Nigerian patterns: BVN (11 digits), NIN, NUBAN account format, phone (+234)
- [ ] Kenyan patterns: M-Pesa format, ID numbers, phone (+254)
- [ ] South African patterns: ID numbers (13 digits), phone (+27)
- [ ] Ghanaian patterns: MoMo references, phone (+233)
- [ ] Generic patterns: IBAN, SWIFT codes, pan-African formats
- [ ] Create pattern test suite with real (anonymized) examples

---

## Phase 2: Secret Detection (Week 2-3)

### Epic 2.1: Credential Patterns
- [ ] API keys: OpenAI (sk-), Anthropic, Google (AIza), AWS
- [ ] Database connection strings (postgres://, mongodb://, mysql://)
- [ ] JWT tokens (eyJ header detection)
- [ ] OAuth tokens and Bearer tokens
- [ ] SSH private keys (BEGIN RSA PRIVATE KEY)
- [ ] Environment variable patterns (API_KEY=, PASSWORD=)

### Epic 2.2: Financial Data Patterns
- [ ] Credit card numbers (Luhn algorithm validation)
- [ ] Payment card CVV patterns
- [ ] Bank account numbers (context-aware: 10-12 digit sequences)
- [ ] Transaction amounts with currency symbols
- [ ] SWIFT/BIC codes

### Epic 2.3: Pattern Configuration System
- [ ] YAML/JSON config file for pattern definitions
- [ ] Pattern severity levels (BLOCK, WARN, LOG)
- [ ] Custom pattern addition without code changes
- [ ] Pattern versioning and updates
- [ ] Validation for pattern regex compilation

---

## Phase 3: API Layer (Week 3-4)

### Epic 3.1: Core Guardrail REST API
- [ ] POST /v1/sanitize endpoint (sync sanitization)
- [ ] Request validation and error handling
- [ ] Response format: {sanitized_prompt, violations[], confidence_score}
- [ ] Rate limiting per API key
- [ ] API versioning strategy (v1)

### Epic 3.2: Health & Monitoring
- [ ] GET /health endpoint (system health check)
- [ ] GET /metrics endpoint (Prometheus format)
- [ ] Pattern statistics (total patterns loaded, cache hit rate)
- [ ] Performance metrics (p50, p95, p99 latency)

### Epic 3.3: Configuration API
- [ ] GET /patterns (list active patterns)
- [ ] POST /patterns/validate (test pattern before deployment)
- [ ] Webhook notification system for violations
- [ ] Admin endpoints with authentication

### Epic 3.4: OpenAI Proxy API
- [ ] POST /v1/openai/chat/completions (proxy endpoint)
- [ ] POST /v1/openai/completions (legacy endpoint)
- [ ] Extract prompt from OpenAI request format
- [ ] Sanitize prompt through guardrail engine
- [ ] Forward sanitized request to OpenAI API
- [ ] Pass-through authentication (Bearer token forwarding)
- [ ] Response streaming support (Server-Sent Events)
- [ ] Error handling and timeout management
- [ ] Add violation headers to response (X-Guardrail-Violations, X-Guardrail-Details)
- [ ] OpenAI-specific request/response logging

### Epic 3.5: Anthropic Proxy API
- [ ] POST /v1/anthropic/messages (proxy endpoint)
- [ ] Extract content from Anthropic request format (handle multiple content blocks)
- [ ] Sanitize text content blocks
- [ ] Preserve image/document blocks unchanged
- [ ] Forward sanitized request to Anthropic API
- [ ] Pass-through API key (x-api-key header)
- [ ] Response streaming support
- [ ] Handle Anthropic-specific headers (anthropic-version)
- [ ] Add violation tracking headers

### Epic 3.6: Google AI Proxy API
- [ ] POST /v1/google/models/:model/generateContent
- [ ] Extract prompt from Google AI request format
- [ ] Sanitize contents array
- [ ] Forward to Google Generative AI API
- [ ] Pass-through API key (x-goog-api-key)
- [ ] Handle Google-specific response format
- [ ] Add violation headers

### Epic 3.7: Proxy Configuration & Behavior
- [ ] Multi-provider configuration system (YAML)
- [ ] Per-provider base URL configuration
- [ ] Per-provider timeout settings
- [ ] Violation behavior modes: block/sanitize/warn
- [ ] Block mode: return 400 with violation details
- [ ] Sanitize mode: clean and forward (default)
- [ ] Warn mode: forward unchanged but log violations
- [ ] Custom headers injection for providers
- [ ] API key stripping from logs (security)
- [ ] Provider health check endpoints

### Epic 3.8: Proxy Middleware & Utilities
- [ ] Generic proxy middleware (reusable for all providers)
- [ ] Request body extraction and modification
- [ ] Response header injection
- [ ] Violation detail formatting for headers
- [ ] Provider-specific request parsers (OpenAI, Anthropic, Google)
- [ ] Provider-specific response formatters
- [ ] Streaming response handler (SSE/chunked transfer)
- [ ] Connection pooling for provider requests
- [ ] Retry logic with exponential backoff

---

## Phase 4: Deployment & Distribution (Week 4-5)

### Epic 4.1: Docker Deployment
- [ ] Multi-stage Dockerfile (optimized binary size)
- [ ] Docker Compose setup with config examples
- [ ] Environment variable configuration
- [ ] Volume mounts for custom patterns
- [ ] Health check configuration
- [ ] Docker Compose with proxy examples (all providers)

### Epic 4.2: Integration Examples - Core API
- [ ] Python: Direct sanitization before OpenAI call
- [ ] Node.js: Express middleware for sanitization
- [ ] Go: Client library example
- [ ] LangChain middleware example
- [ ] Direct HTTP client examples (curl)

### Epic 4.3: Integration Examples - Proxy Mode
- [ ] Python: OpenAI SDK with proxy base_url
- [ ] Python: Anthropic SDK with proxy base_url
- [ ] Node.js: OpenAI library with proxy configuration
- [ ] LangChain: Custom LLM wrapper using proxy
- [ ] Vercel AI SDK: Proxy configuration example
- [ ] Environment variable configuration templates
- [ ] Docker Compose: App + Guardrail proxy setup
- [ ] Kubernetes: Sidecar proxy deployment example

### Epic 4.4: Documentation
- [ ] Architecture overview diagram (include proxy flow)
- [ ] Quick start guide (5 minutes to running)
- [ ] Proxy mode quick start (change one line)
- [ ] Pattern customization guide
- [ ] API reference documentation (core + proxy endpoints)
- [ ] Deployment guide (Docker, Kubernetes, bare metal)
- [ ] Integration tutorials (with code samples)
- [ ] Proxy vs Core API decision guide
- [ ] Network-level proxy deployment guide (for IT teams)

---

## Phase 5: Observability & Alerting (Week 5-6)

### Epic 5.1: Logging System
- [ ] Structured logging (JSON format)
- [ ] Log levels (DEBUG, INFO, WARN, ERROR)
- [ ] Violation logging (anonymized by default)
- [ ] Audit trail for blocked requests
- [ ] Log rotation and retention configuration
- [ ] Separate proxy request logs (which provider, latency)
- [ ] API key redaction in all logs

### Epic 5.2: Alerting System
- [ ] Webhook integration for violations
- [ ] Email notification support (SMTP)
- [ ] Slack webhook integration
- [ ] Alert batching (don't spam on high volume)
- [ ] Alert severity routing
- [ ] Provider-specific alerts (OpenAI violations vs Anthropic)
- [ ] Alert templates with violation details

### Epic 5.3: Metrics & Analytics
- [ ] Prometheus metrics endpoint expansion
- [ ] Core API metrics: requests/sec, latency, violations
- [ ] Proxy metrics: per-provider requests, latency, errors
- [ ] Violation metrics by type, severity, provider
- [ ] Pattern match performance metrics
- [ ] JSON export of violation statistics
- [ ] CSV export for compliance reports
- [ ] Time-series data format for graphing
- [ ] Aggregation by violation type, severity, time, provider

---

## Phase 6: Testing & Hardening (Week 6-7)

### Epic 6.1: Performance Testing
- [ ] Benchmark tests for pattern matching (target: <5ms p95)
- [ ] Load testing core API (1000 concurrent requests)
- [ ] Load testing proxy endpoints (simulate real traffic)
- [ ] End-to-end latency testing (client → proxy → provider)
- [ ] Memory profiling and optimization
- [ ] Identify and fix bottlenecks
- [ ] Document performance characteristics
- [ ] Streaming performance tests

### Epic 6.2: Security Hardening
- [ ] Input validation fuzzing
- [ ] ReDoS (Regular Expression DoS) prevention
- [ ] Pattern injection attack prevention
- [ ] Secrets management for webhooks/alerts
- [ ] Security audit of dependencies
- [ ] API key handling security review
- [ ] Proxy authentication bypass testing
- [ ] Provider credential leakage prevention

### Epic 6.3: Edge Cases & Validation
- [ ] Test with encoded inputs (base64, URL encoding, hex)
- [ ] Test with obfuscated patterns
- [ ] Test with large prompts (100KB+)
- [ ] Test with Unicode and non-ASCII text
- [ ] False positive rate measurement and tuning
- [ ] Proxy: Test with malformed provider requests
- [ ] Proxy: Test streaming interruptions
- [ ] Proxy: Test provider timeout scenarios
- [ ] Proxy: Test with concurrent requests to multiple providers

### Epic 6.4: Integration Testing
- [ ] End-to-end tests with real OpenAI API
- [ ] End-to-end tests with real Anthropic API
- [ ] End-to-end tests with real Google AI API
- [ ] Test violation blocking in proxy mode
- [ ] Test sanitization pass-through in proxy mode
- [ ] Test warning mode behavior
- [ ] Test response header injection
- [ ] Verify API key forwarding works correctly

---

## Phase 7: Launch Preparation (Week 7-8)

### Epic 7.1: Compliance Mapping
- [ ] Map patterns to NDPR (Nigeria Data Protection Regulation)
- [ ] Map patterns to POPIA (South Africa)
- [ ] Map patterns to GDPR (for international users)
- [ ] Map patterns to PCI-DSS (payment cards)
- [ ] Create compliance report template

### Epic 7.2: Pre-built Configurations
- [ ] Nigerian banking template (patterns + proxy config)
- [ ] Kenyan fintech template
- [ ] South African enterprise template
- [ ] Generic African business template
- [ ] Strict compliance mode (maximum protection)
- [ ] Developer mode (warnings only)
- [ ] Proxy-first configuration (all providers enabled)
- [ ] Core API-only configuration (proxy disabled)

### Epic 7.3: Community Setup
- [ ] Contributing guidelines (CONTRIBUTING.md)
- [ ] Code of conduct
- [ ] Issue templates (bug, feature request, pattern suggestion, provider request)
- [ ] PR template
- [ ] Roadmap document (public)

### Epic 7.4: Marketing Assets
- [ ] Demo video: Core API usage
- [ ] Demo video: Proxy mode (one-line change)
- [ ] Before/after examples (sanitized prompts)
- [ ] Blog post: "Why we built this"
- [ ] Blog post: "Zero-code AI security with proxy mode"
- [ ] GopherCon Africa talk preparation (tie-in)
- [ ] Twitter thread template
- [ ] Dev.to/Hashnode article draft
- [ ] Comparison chart: Core API vs Proxy mode
- [ ] Network diagram: Proxy as infrastructure

---

## Phase 8: Post-Launch (Week 8+)

### Epic 8.1: Monitoring Adoption
- [ ] Track GitHub stars, forks, clones
- [ ] Monitor issue submissions
- [ ] Community engagement metrics
- [ ] Docker Hub pull statistics
- [ ] Integration examples usage
- [ ] Proxy vs Core API adoption ratio
- [ ] Most popular provider in proxy mode

### Epic 8.2: Iteration Based on Feedback
- [ ] Weekly review of issues and PRs
- [ ] Pattern library updates based on real usage
- [ ] Performance improvements from user reports
- [ ] Documentation gaps identified and filled
- [ ] New provider requests for proxy mode
- [ ] Provider-specific bug fixes

### Epic 8.3: Provider Expansion
- [ ] Cohere proxy API
- [ ] Mistral AI proxy API
- [ ] Azure OpenAI proxy API
- [ ] Hugging Face Inference API proxy
- [ ] Replicate API proxy
- [ ] Generic OpenAI-compatible proxy (Ollama, LocalAI, etc.)

### Epic 8.4: Future Enhancements (Backlog)
- [ ] Optional SLM module for contextual detection
- [ ] ONNX Runtime integration for Go-native ML
- [ ] Web dashboard for hosted version
- [ ] API key management system
- [ ] Team collaboration features
- [ ] Enterprise on-prem installer
- [ ] GraphQL proxy support
- [ ] gRPC proxy support
- [ ] Request/response caching
- [ ] Cost analytics per provider

---

## Success Metrics (Track Weekly)

**Technical:**
- [ ] Core API latency p95 < 5ms
- [ ] Proxy end-to-end latency p95 < 50ms (including provider)
- [ ] Pattern detection accuracy > 95%
- [ ] False positive rate < 2%
- [ ] Zero-downtime deployment capability
- [ ] Streaming response latency overhead < 10ms

**Adoption:**
- [ ] 50 GitHub stars in first month
- [ ] 10 production deployments (core or proxy)
- [ ] 5 community-contributed patterns
- [ ] 3 integration examples from community
- [ ] 50% of deployments using proxy mode

**Brand (OpenRay):**
- [ ] 100 mentions on Twitter
- [ ] 3 blog posts from external developers
- [ ] Featured in 1 African tech newsletter
- [ ] GopherCon Africa talk delivered successfully
- [ ] "Proxy mode" becomes recognized term

---

## Priority Labels for Linear

**P0 - Critical:** Core detection, Core API, OpenAI proxy, Docker deployment
**P1 - High:** Anthropic proxy, African patterns, documentation, proxy examples
**P2 - Medium:** Google proxy, alerting, dashboard export, compliance mapping
**P3 - Low:** Additional providers, advanced features, optional integrations

**Tags:** `backend`, `api`, `proxy`, `patterns`, `security`, `docs`, `devops`, `testing`, `community`, `provider-integration`

---

## Proxy-Specific Testing Checklist

Before launch, verify all proxy endpoints:

**OpenAI Proxy:**
- [ ] Chat completions (non-streaming)
- [ ] Chat completions (streaming)
- [ ] Legacy completions endpoint
- [ ] Function calling preserved
- [ ] Vision inputs preserved
- [ ] Violation headers added correctly

**Anthropic Proxy:**
- [ ] Messages API (non-streaming)
- [ ] Messages API (streaming)
- [ ] Multiple content blocks handled
- [ ] Image content preserved
- [ ] System prompts sanitized
- [ ] Tool use preserved

**Google AI Proxy:**
- [ ] Generate content endpoint
- [ ] Multi-turn conversations
- [ ] Safety settings preserved
- [ ] Generation config preserved

**All Providers:**
- [ ] Authentication forwarding works
- [ ] Error messages from providers passed through
- [ ] Rate limits from providers respected
- [ ] Timeouts configured correctly
- [ ] Violation logging accurate
- [ ] Performance metrics collected

---

## Updated README Sections to Add

### Proxy Mode Quick Start
````markdown
## ⚡ Fastest Way to Get Started: Proxy Mode

Change **one line** of code to secure all your AI API calls:

**Python (OpenAI):**
```python
from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:8080/v1/openai"  # ← Only change needed!
)

# Use exactly as before - sanitization is automatic
response = client.chat.completions.create(
    model="gpt-4",
    messages=[{"role": "user", "content": "Your prompt here"}]
)
```

**Python (Anthropic):**
```python
import anthropic

client = anthropic.Anthropic(
    base_url="http://localhost:8080/v1/anthropic"  # ← Only change needed!
)
```

**Environment Variables (Works with any library):**
```bash
export OPENAI_BASE_URL="http://localhost:8080/v1/openai"
export ANTHROPIC_BASE_URL="http://localhost:8080/v1/anthropic"
```

That's it. Every API call now goes through Guardrail
````

Want me to create separate detailed documentation for the proxy implementation, including request/response flow diagrams and provider-specific considerations?