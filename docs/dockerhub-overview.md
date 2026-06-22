<!--
Docker Hub repository overview (the `full_description` for openray/eko).
Intentionally a Hub-specific subset of README.md — README.md is canonical;
sync changes here deliberately. HTML comments are not rendered on the Hub page.
-->

# 🛡️ Ekō

**Open-source, context-aware security proxy that prevents leaking sensitive data to AI services — with first-class detectors for African regulated domains.**

Ekō sits between your application and an AI provider, detects sensitive data in prompts, and either **redacts** or **tokenizes** it before it leaves your environment.

🎮 **[Try the live playground →](https://ek-playground.openray.workers.dev/)** &nbsp;·&nbsp; 📦 **[Source & full docs on GitHub →](https://github.com/Openray-ai/eko)**

---

## 🚀 Quick start

```bash
docker run --rm -p 8080:8080 openray/eko:main-latest
```

Verify it is up:

```bash
curl http://localhost:8080/health
curl http://localhost:8080/ready
```

Sanitize a prompt:

```bash
curl -X POST http://localhost:8080/v1/sanitize \
  -H "Content-Type: application/json" \
  -d '{"prompt":"My BVN is 12345678901 and email is john@company.com"}'
```

---

## 🏷️ Supported tags & architectures

| Tag | Description |
| --- | --- |
| `main-latest` | Latest build from the `main` branch |
| `latest` | Same image as `main-latest` |
| `sha-<commit>` | Immutable build pinned to a specific commit |

**Architectures:** multi-arch manifest — `linux/amd64` and `linux/arm64` (runs natively on Apple Silicon / ARM servers and x86).

---

## 🔌 OpenAI-compatible multi-provider proxy

Point the OpenAI SDK at Ekō — only the base URL changes:

```bash
export OPENAI_BASE_URL="http://localhost:8080/v1"
```

Routes: `POST /v1/chat/completions`, `POST /v1/responses`, plus the core `POST /v1/sanitize`. Ekō forwards the **sanitized** request to the upstream selected by model routing, with built-in support for OpenAI, Anthropic, Gemini, and DeepSeek model families.

---

## 🎨 Run with your own configuration

```bash
docker run -d \
  -p 8080:8080 \
  -v $(pwd)/configs/config.yaml:/app/configs/config.yaml:ro \
  -v $(pwd)/patterns:/app/patterns \
  openray/eko:main-latest
```

Start from `configs/config.example.yaml` in the repo. For a Redis-backed token vault and the optional contextual SLM sidecar, use Docker Compose — see the [GitHub README](https://github.com/Openray-ai/eko).

> **Secrets:** never bake `local_master_key`, Vault tokens, or provider API keys into the image — inject them at deploy time via your secrets manager.

---

## 🌍 What gets detected

- **Credentials & secrets** — API keys (OpenAI, Anthropic, Google, AWS, Azure), database connection strings, JWT/OAuth, SSH keys, env-style secrets (`API_KEY=`, `PASSWORD=`)
- **PII** — Nigerian (BVN, NIN, NUBAN, +234), Kenyan (M-Pesa, ID, +254), South African (ID, +27), Ghanaian (Mobile Money, +233), plus emails, IBAN, SWIFT/BIC
- **Financial** — credit card numbers, bank accounts, transaction references
- **Custom business patterns** — your own regex rules under `patterns/custom`

Optional contextual detection of person names and addresses via an opt-in Small Language Model sidecar.

---

## 📊 Monitoring

Prometheus-compatible metrics at `GET /metrics`; liveness and readiness at `/health` and `/ready`. A container `HEALTHCHECK` is built in.

---

## 🔗 Links

- 📦 **GitHub:** https://github.com/Openray-ai/eko
- 🎮 **Playground:** https://ek-playground.openray.workers.dev/
- 🌍 **OpenRay:** https://openray.ai
- 📜 **License:** MIT

---

<sub>Ekō (Yoruba): "to guard, to protect" — built with ❤️ in Africa, for the world.</sub>
