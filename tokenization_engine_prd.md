Ekō Tokenization Engine (Reasoning-Preserving Sanitization)

## 1. Background & Motivation

### Current State

Ekō currently performs **regex-based detection + redaction** of sensitive data via:

* `detector.Detect(input)` → `[]Violation`
* `sanitizer.Sanitize(input)` → redacted string + violations
* Proxies (OpenAI today) forward **sanitized text** to LLM providers

Redaction replaces sensitive spans with static markers (e.g. `[REDACTED_BVN]`).

### Problem

While redaction prevents data leakage, it **degrades LLM reasoning quality**:

* Referential integrity is lost (`[REDACTED]` collapses identity)
* Symbolic reasoning breaks (LLMs can’t track entities)
* Multi-step debugging, analysis, and correlation suffer
* Responses become vague or incorrect (“dumb responses”)

This is unacceptable for developer workflows and enterprise analysis use cases.

---

## 2. Goal

Introduce a **tokenization-based sanitization mode** that:

* Prevents sensitive data from leaving Ekō
* Preserves semantic structure for the LLM
* Allows **reversible resolution** after the LLM response
* Is **safe against token injection, drift, and replay**
* Fits cleanly into the existing `detector → sanitizer → proxy` pipeline

---

## 3. Non-Goals

This PRD explicitly does **not** include:

* ML-based or contextual detection
* Cross-request memory or long-term vaulting
* Persistent identity tracking across sessions
* Dashboard or UI features
* Multi-provider token unification (OpenAI only initially)

---

## 4. High-Level Design

### Current Flow

```
Client → Ekō → [redaction] → LLM → Client
```

### New Flow (Tokenization Mode)

```
Client
  ↓
Ekō
  ├── Detect violations
  ├── Replace sensitive spans with opaque tokens
  ├── Store token ↔ value mapping (request-scoped)
  ↓
LLM (sees only tokens)
  ↓
Ekō
  ├── Validate tokens in response
  ├── Resolve tokens back to original values
  ├── Reject or sanitize invalid token usage
  ↓
Client
```

Ekō becomes a **semantic boundary**, not just a filter.

---

## 5. Tokenization Model

### Token Properties

All tokens MUST be:

* **Opaque** (non-guessable)
* **Namespaced**
* **Typed**
* **Request-scoped**
* **Ephemeral (TTL-bound)**

#### Example Token

```
⟦eko:pii:bvn:K9f2A7⟧
```

Components:

* `eko` → namespace (prevents collision)
* `pii:bvn` → type + subtype
* `K9f2A7` → cryptographically random ID

---

## 6. Token Lifecycle

### 6.1 Creation

During sanitization:

1. `detector.Detect` returns violations with spans
2. For eligible violations:

   * Generate token
   * Replace span with token
   * Store mapping in a **Token Vault**

### 6.2 Storage

Token mappings live:

* **In memory**
* **Request-scoped**
* **Not logged**
* **Not serialized**
* Optional: encrypted at rest (future)

Proposed structure:

```go
type TokenEntry struct {
	Token        string
	Original     string
	Type         string
	Pattern      string
	CreatedAt    time.Time
	ExpiresAt    time.Time
}
```

### 6.3 Resolution

On LLM response:

1. Scan response for Ekō tokens
2. Validate:

   * Token was issued in this request
   * Token type matches expected context
3. Replace tokens with original values
4. Reject or sanitize response if:

   * Unknown token appears
   * Token type mismatch
   * Token count exceeds issued count

---

## 7. Integration with Existing Code

### 7.1 Sanitizer Changes

#### New Mode

Add a configurable sanitization mode:

```go
type SanitizationMode string

const (
  RedactMode   SanitizationMode = "redact"
  TokenizeMode SanitizationMode = "tokenize"
)
```

Extend `Sanitizer`:

```go
type Sanitizer struct {
  detector *detector.Detector
  mode     SanitizationMode
  tokenizer *Tokenizer // new
}
```

#### New Result Type (Backwards Compatible)

```go
type Result struct {
  OriginalPrompt   string
  SanitizedPrompt  string
  Violations       []detector.Violation
  Safe             bool
  ProcessingTimeMs float64
  RedactedCount    int

  TokensUsed       int              // NEW
}
```

No breaking API changes.

---

### 7.2 Tokenizer Component (New)

```
internal/core/tokenizer/
  ├── tokenizer.go
  ├── vault.go
  ├── resolver.go
  └── errors.go
```

Responsibilities:

* Token generation
* Mapping storage
* Safe replacement
* Validation and resolution

---

### 7.3 Proxy Changes (OpenAI)

#### Request Path

* Replace `Sanitize(msg.Content)` with:

  * `SanitizeWithContext(ctx, msg.Content)`
* Store token vault in **gin.Context**

#### Response Path

Before returning response:

* Resolve tokens
* Validate integrity
* Apply fallback behavior on violation

---

## 8. Configuration

Extend `ProxyConfig.Behavior`:

```yaml
proxy:
  behavior:
    on_violation: sanitize
    sanitization_mode: tokenize # redact | tokenize
    token_ttl_ms: 30000
    allow_token_resolution: true
```

Defaults:

* `sanitization_mode: redact`
* Tokenization is **opt-in**

---

## 9. Security & Threat Model

### 9.1 Token Injection

Mitigation:

* Strict token prefix (`⟦eko:`)
* Random IDs
* Reject tokens not in vault

### 9.2 Token Drift / Hallucination

Mitigation:

* Only resolve known tokens
* Strip or error on unknown tokens

### 9.3 Cross-Request Leakage

Mitigation:

* Vault is request-scoped
* Cleared after response
* No global storage

### 9.4 Replay Attacks

Mitigation:

* TTL enforcement
* Context-bound validation

---

## 10. Violation Policy Matrix

| Data Type            | Default Action |
| -------------------- | -------------- |
| API Keys             | Redact / Block |
| Passwords            | Redact         |
| Private Keys         | Block          |
| BVN / Account IDs    | Tokenize       |
| Emails / Phones      | Tokenize       |
| Business Identifiers | Tokenize       |

This mapping is configurable.

---

## 11. Observability

### Headers (unchanged + extended)

* `X-Eko-Violations-Found`
* `X-Eko-Redacted-Count`
* `X-Eko-Tokens-Issued` **(NEW)**

### Logs

* Token counts only
* Never log token contents or originals

---

## 12. Rollout Plan

### Phase 1 (Experimental)

* OpenAI proxy only
* Non-streaming only
* Opt-in via config
* Extensive tests

### Phase 2

* Streaming-safe token resolution
* Anthropic support
* Policy tuning

---

## 13. Success Metrics

* No sensitive data leakage (zero regression)
* LLM response quality improves in tokenized mode
* p95 latency increase ≤ 5ms
* Zero token injection incidents in tests

---

## 14. Why This Matters

This feature moves Ekō from:

> “AI prompt redaction tool”

to:

> **“Semantic security boundary for LLMs.”**

Very few systems attempt **reversible, reasoning-preserving sanitization** safely.
This is a **strong technical differentiator** and aligns with serious enterprise usage.
