# Token-Based De-Identification (Phase 1)

## Product Name

**Ekō Token-Based De-Identification — Phase 1**

---

## 1. Problem Statement

Ekō currently protects sensitive data by **static redaction** (e.g. `[REDACTED_BVN]`).
While effective for security, this approach **destroys semantic structure**, causing:

- LLMs to lose the ability to:
  - distinguish between multiple identifiers
  - reason about relationships
  - track entity references across prompts

- noticeably degraded response quality
- users bypassing Ekō to regain model usefulness (security failure)

**Conclusion:** Redaction-only sanitization is correct but insufficient for real-world LLM workflows.

---

## 2. Solution Overview

Introduce **token-based de-identification** using **format-preserving tokens**.

Instead of replacing sensitive values with static markers, Ekō replaces them with **synthetic but valid-looking values** that preserve:

- type
- length
- format
- relative uniqueness

The LLM reasons over realistic data, but **never sees the original values**.

After the LLM responds, Ekō **re-identifies** the tokens back to the original values **inside the trust boundary**.

---

## 3. Phase 1 Scope (Strict)

### Included

- Core tokenizer engine
- Session-scoped token vault with TTL
- Non-streaming OpenAI proxy integration
- Format-preserving token generators
- Deterministic reuse of tokens within a session
- Credential hard-redaction (never tokenized)
- Config-gated opt-in behavior

### Explicitly Excluded

- Streaming re-identification
- SSE rewriting
- Anthropic / Google proxies
- Persistent storage
- Cross-session token reuse
- Pattern-level strategy overrides

---

## 4. Key Decisions (Non-Negotiable)

| Decision                    | Rationale                      |
| --------------------------- | ------------------------------ |
| Format-preserving tokens    | Preserve LLM reasoning quality |
| Credentials always redacted | Tokenizing secrets is unsafe   |
| Default mode = `redact`     | Backward compatibility         |
| Tokenization is opt-in      | Safe rollout                   |
| Session-scoped vaults       | Prevent cross-request leakage  |
| Non-streaming only          | Reduce complexity in Phase 1   |

---

## 5. Architecture Changes

### 5.1 New Package: `internal/core/tokenizer/`

| File           | Responsibility               |
| -------------- | ---------------------------- |
| `errors.go`    | Tokenization-specific errors |
| `vault.go`     | TokenMap + VaultManager      |
| `tokenizer.go` | Token generation interfaces  |
| `resolver.go`  | Response token resolution    |

---

### 5.2 Modified Components

#### `internal/config/config.go`

Add to `BehaviorConfig`:

```go
SanitizationMode string `yaml:"sanitization_mode"` // redact | tokenize
TokenTTLms       int    `yaml:"token_ttl_ms"`
```

---

#### `internal/core/sanitizer/sanitizer.go`

Add **context-aware API**:

```go
SanitizeWithSession(
  input string,
  sessionID string,
) (*Result, error)
```

Responsibilities:

- call detector (unchanged)
- branch behavior based on `SanitizationMode`
- delegate token generation to tokenizer
- delegate storage to vault manager

---

#### `internal/proxy/openai/openai.go`

Changes:

- Extract `session_id` from `X-Eko-Session-ID`
- Generate one if missing
- Call `SanitizeWithSession`
- After non-streaming OpenAI response:
  - call `Resolver.ResolveResponse`

- Add tokenization headers

---

#### `internal/proxy/common/proxy.go`

Extend `BaseProxy`:

```go
type BaseProxy struct {
  sanitizer *sanitizer.Sanitizer
  resolver  *tokenizer.Resolver
}
```

---

#### `internal/api/handlers/sanitize.go`

- Accept optional `session_id`
- Return `session_id` in response if generated

---

#### `cmd/eko/main.go`

- Wire tokenizer only if `sanitization_mode == tokenize`
- Otherwise preserve existing behavior

---

## 6. Token Vault Design

### 6.1 Vault Scope

- **Keyed by session ID**
- **TTL-based eviction**
- In-memory only
- Thread-safe
- No persistence

---

### 6.2 Structures

```go
type TokenMap struct {
  forward map[string]string // original → token
  reverse map[string]string // token → original
}

type Vault struct {
  sessionID string
  expiresAt time.Time
  tokens    TokenMap
}

type VaultManager struct {
  vaults map[string]*Vault
}
```

---

### 6.3 Lifecycle

| Step                | Action                      |
| ------------------- | --------------------------- |
| Request start       | Get or create vault         |
| Token generation    | Store bidirectional mapping |
| Response resolution | Lookup token → original     |
| TTL expiry          | Vault destroyed             |

---

## 7. Token Generation Rules

### 7.1 Tokenization Eligibility

| Type                  | Behavior           |
| --------------------- | ------------------ |
| Credentials           | ❌ Always redacted |
| PII                   | ✅ Tokenized       |
| Financial identifiers | ✅ Tokenized       |
| Business identifiers  | ✅ Tokenized       |
| LOG severity          | No replacement     |

---

### 7.2 Token Format (Examples)

| Data Type   | Original                              | Token                                                 |
| ----------- | ------------------------------------- | ----------------------------------------------------- |
| Email       | [john@acme.com](mailto:john@acme.com) | [user_a7f3@example.com](mailto:user_a7f3@example.com) |
| BVN         | 12345678901                           | 00000000001                                           |
| Phone       | +2348012345678                        | +000-000-000-0001                                     |
| Account     | 0123456789                            | 0000000001                                            |
| Credit Card | 4532015112830366                      | 0000000000000001                                      |
| SA ID       | 8001015009087                         | 0000000000001                                         |
| IBAN        | GB29NWBK60161331926819                | XX00XXXX00000001                                      |
| API Key     | sk-proj-abc123                        | `[REDACTED_API_KEY]`                                  |
| Password    | admin123                              | `[REDACTED_PASSWORD]`                                 |

**Rules**

- Length preserved
- Character class preserved
- Prefixes preserved when meaningful
- Deterministic reuse within a session

---

## 8. Request Lifecycle (Tokenize Mode)

```
Client
  |
  |  X-Eko-Session-ID (optional)
  v
OpenAI Proxy
  |
  |-- determine session ID
  |-- Sanitizer.SanitizeWithSession()
        |
        |-- Detector.Detect()
        |-- tokenize violations
              |
              |-- VaultManager.GetOrCreate()
              |-- Generate / reuse tokens
  |
Forward sanitized request to OpenAI
  |
Receive non-streaming response
  |
Resolver.ResolveResponse()
  |
Return original values to client
```

---

## 9. Resolver Behavior

### 9.1 Resolution Rules

- Replace **only tokens present in the vault**
- Unknown tokens:
  - ignored (default)
  - counted for metrics

- Resolution happens **after full response read**
- No partial or streaming resolution in Phase 1

---

## 10. Headers & Observability

### Response Headers

| Header                    | Meaning            |
| ------------------------- | ------------------ |
| `X-Eko-Sanitization-Mode` | redact / tokenize  |
| `X-Eko-Tokens-Issued`     | count              |
| `X-Eko-Session-ID`        | session identifier |
| `X-Eko-Violations-Found`  | unchanged          |

---

## 11. Configuration

```yaml
proxy:
  behavior:
    sanitization_mode: "tokenize" # default: redact
    token_ttl_ms: 30000 # 30s
```

If `X-Eko-Session-ID` is absent:

- Ekō generates a request-scoped ID
- Returned to client for follow-up calls

---

## 12. Security Guarantees

| Risk                  | Mitigation                          |
| --------------------- | ----------------------------------- |
| Token injection       | Resolver only resolves known tokens |
| Cross-request leakage | Session-scoped vaults               |
| Secret exfiltration   | Credentials never tokenized         |
| Replay attacks        | TTL-based vault eviction            |

---

## 13. Success Criteria

### Functional

- LLM output quality improves vs redaction
- No credentials ever leave Ekō
- Deterministic token reuse within session

### Non-Functional

- <5ms overhead per request (non-stream)
- No shared mutable global state
- No breaking change to existing clients

---

## 14. Phase 2 (Explicitly Deferred)

- Streaming re-identification (Aho-Corasick + sliding window)
- Anthropic / Google support
- Pattern-level tokenization strategies
- Structured response awareness