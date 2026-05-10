# Security Policy

Ekō is a security-positioned project that handles credentials, PII, and regulated financial data on the request path. We take vulnerability reports seriously and appreciate the effort it takes to investigate and disclose them responsibly.

## Supported versions

Ekō is currently pre-1.0. Security fixes are applied to the latest `develop` and the most recent tagged release. Older releases are not patched.

| Version | Supported |
| ------- | --------- |
| `develop` (latest) | Yes |
| Latest release tag | Yes |
| Older tags | No |

## Reporting a vulnerability

**Please do not file public GitHub issues for security vulnerabilities.**

Report privately via GitHub Security Advisories:

> [Report a vulnerability](https://github.com/Openray-ai/eko/security/advisories/new)

Include as much of the following as you can:

- A description of the issue and its impact
- Steps to reproduce, ideally with a minimal proof of concept
- The affected version, commit, or deployment mode (core API, OpenAI proxy, SLM sidecar)
- Any suggested mitigation

You will receive an acknowledgement within **3 business days**. We aim to provide an initial assessment within **7 business days** and a fix or mitigation timeline shortly after, depending on severity.

## Scope

In scope:

- The Ekō Go server (`cmd/eko`, `internal/`, `pkg/`)
- The SLM sidecar (`slm-sidecar/`)
- Default detection patterns (`patterns/`)
- Official Docker images and deployment manifests in this repo

Out of scope:

- Vulnerabilities in upstream dependencies (please report to the upstream project; we will track and update)
- Issues in user-supplied custom patterns or configuration
- Findings that require access to a host already compromised at the OS level
- Social engineering, physical attacks, or denial-of-service via raw traffic volume

## Safe harbor

We will not pursue legal action against researchers who:

- Make a good-faith effort to follow this policy
- Avoid privacy violations, data destruction, and service degradation
- Give us reasonable time to remediate before public disclosure
- Do not access or modify data that does not belong to them

## Coordinated disclosure

Once a fix is available, we will coordinate a disclosure window with the reporter and credit them in the advisory unless they prefer to remain anonymous.
