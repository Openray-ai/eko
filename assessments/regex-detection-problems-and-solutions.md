# Regex Detection: Problems and Proposed Solutions

## Scope
This document logs observed detection issues and proposed fixes only.
No new implementation changes are introduced by this note.

## Problem 1: API key coverage drift (high recall vs precision)

### Observed issue
Current key detection relies on strict provider regexes. This misses newer/variant key formats (e.g. evolving provider prefixes), while broad regexes can introduce false positives.

### Why it happens
- Provider formats evolve over time.
- A single rigid regex for each provider can become stale.
- Overly generic regexes increase noisy matches.

### Proposed solution
Use a layered detection strategy:
1. Keep strict provider-specific regexes as `BLOCK` (high precision).
2. Add constrained variant-aware provider regexes (e.g. `sk-proj`, `sk-live`, `sk-test`) as `BLOCK`.

This should not be implemented, as it will cause regex drift. But we solved this by allowing custom patterns to be added by individual organizations using the custom patterns directory.

3. Add generic secret-assignment patterns as `WARN` only.
4. Add lightweight validators before escalation (`length`, `charset mix`, denylist such as `test`, `dummy`, `example`).
5. Maintain a regression corpus (true positive + false positive samples) and run it in CI.

### Acceptance criteria
- Known provider formats are detected consistently.
- Generic patterns improve recall without materially increasing false positives.
- CI fails if known key formats regress.

## Problem 2: SWIFT false positives inside long uppercase strings

### Observed issue
A long OpenAI-style key payload was partially matched as multiple `swift_code` violations.

### Example behavior seen
Input included a long `sk-proj-...` token, and detector returned `swift_code` matches from internal uppercase substrings.

### Why it happens
- SWIFT regex was too permissive for contiguous uppercase alphanumeric sequences.
- Pattern lacked strict BIC structure and boundary controls.

### Proposed solution
Tighten SWIFT/BIC matching to valid structure and boundaries:
- Use strict institution + country + location (+ optional branch) shape.
- Enforce word boundaries to reduce substring matches inside larger tokens.

Recommended form:
- `\\b[A-Z]{4}[A-Z]{2}[A-Z0-9]{2}(?:[A-Z0-9]{3})?\\b`

### Acceptance criteria
- Valid SWIFT/BIC values still match.
- Long non-SWIFT key payloads no longer trigger `swift_code` detections.

## Problem 3: AWS temporary key (`ASIA...`) misclassified as SWIFT

### Observed issue
Input:
- `aws_temp=ASIAQWERTYUIOPLKJHG`

Output showed:
- `swift_code` matches: `ASIAQWERTYU` and `IOPLKJHG`
- tokenization applied to those false matches

### Why it happens
- The current SWIFT matcher is still permissive enough to treat contiguous uppercase sequences as BIC-like tokens.
- AWS temporary credential prefixes (e.g., `ASIA`) are not explicitly captured as credential patterns, so overlap resolution may keep the wrong class of match.

### Proposed solution
1. Add explicit AWS temporary key pattern coverage:
   - `\\b(?:AKIA|ASIA)[0-9A-Z]{16}\\b`
2. Keep SWIFT strict and bounded to valid BIC structure with word boundaries.
3. Prioritize credential-class detections over financial lookalikes when ranges overlap.

### Acceptance criteria
- `ASIA...` and `AKIA...` keys are detected as `credential`.
- No `swift_code` violations are produced for standalone AWS key strings.

## Problem 4: Credit card with spaces not detected

### Observed issue
Input:
- `card=4111 1111 1111 1111`
- `card=4111-1111-1111-1111`

Output showed:
- no violations (`safe: true`)
- value passed through unchanged

### Why it happens
- Current card pattern only matches contiguous digits (`13-19`), so formatted PAN values with spaces are missed.
- No normalization step (remove spaces/hyphens before validation) is applied prior to matching.

### Proposed solution
1. Extend card detection to include grouped formats with separators (spaces/hyphens).
2. Normalize candidate PAN strings (strip separators) before length/Luhn checks.
3. Keep strict post-validation to avoid broad false positives.

### Acceptance criteria
- Common PAN formats like `4111 1111 1111 1111` and `4111-1111-1111-1111` are detected.
- Random grouped numbers without valid card characteristics are not over-flagged.

## Problem 5: URL-encoded email not detected (`%40` for `@`)

### Observed issue
Input:
- `email=john.doe%40example.com`

Output showed:
- no violations (`safe: true`)
- value passed through unchanged

### Why it happens
- Email detector expects canonical `@` format.
- URL-encoded representation (`%40`) is semantically the same data but not normalized before regex matching.

### Proposed solution (without regex drift)
1. Add a pre-detection canonicalization layer:
   - Percent-decode once on a copy of input.
   - Detect on both raw and canonical forms.
   - Merge/dedupe violations.
2. Keep email regex strict; do not expand it to encode every transport variant.
3. Add guardrails:
   - Decode only valid `%HH` sequences.
   - Single-pass decode only (no recursive decode chains).
   - Optional allowlist for decoded symbols relevant to identifiers (`@`, `.`, `+`, `-`, `_`).
4. Preserve replacement correctness:
   - Maintain offset mapping between canonical and original strings so sanitization edits original text at correct positions.

### Acceptance criteria
- `john.doe%40example.com` is detected and sanitized.
- Existing canonical email detection behavior remains unchanged.
- No meaningful increase in false positives from regex broadening.

## Problem 6: Unicode confusable email characters bypass detection

### Observed issue
Input:
- `email=jоhn.doe@exаmple.com` (contains Cyrillic lookalike letters)

Output showed:
- no violations (`safe: true`)
- value passed through unchanged

### Why it happens
- Regex classes are ASCII-oriented for email structure.
- Unicode confusables (homoglyphs) visually resemble ASCII but are different code points.

### Proposed solution (without regex drift)
1. Add a canonicalization stage before detection:
   - Normalize Unicode (NFKC) on a detection copy.
   - Apply confusable skeleton mapping for high-risk contexts (email/usernames/domains).
2. Detect on both raw and canonicalized strings, then merge/dedupe.
3. Keep email regex strict and stable; do not balloon character classes to chase scripts.
4. Add safety constraints:
   - Restrict aggressive confusable folding to specific token classes (email/domain) to avoid semantic overreach.
   - Preserve original text for output replacement via offset mapping.

### Acceptance criteria
- Confusable variants like `jоhn.doe@exаmple.com` are detected as email.
- Legitimate non-email multilingual text is not over-flagged by global folding.
- Canonical ASCII email detection remains unchanged.

## Problem 7: Phone numbers with separators/spaces bypass detection

### Observed issue
Input:
- `phone=+234 801 234 5678`

Output showed:
- no violations (`safe: true`)
- value passed through unchanged

### Why it happens
- Current phone patterns expect contiguous digits after country/local prefix.
- Human-entered phone formats often include spaces, hyphens, or parentheses.

### Proposed solution (without regex drift)
1. Add phone canonicalization before detection:
   - Build a normalized detection copy that removes allowed phone separators (`space`, `-`, `(`, `)`).
   - Preserve leading `+` and digits.
2. Detect on both raw and normalized views, then merge/dedupe.
3. Keep existing phone regexes strict; avoid broad permissive regex expansion.
4. Use offset mapping to sanitize correctly in original text when match came from normalized view.

### Acceptance criteria
- `+234 801 234 5678` and similar separator variants are detected and sanitized.
- Existing compact-number detection behavior remains unchanged.
- No broad false-positive increase from permissive regex changes.

## Problem 8: IBAN with lowercase and spaces bypasses detection

### Observed issue
Input:
- `iban=gb29 nwbk 6016 1331 9268 19`

Output showed:
- no violations (`safe: true`)
- value passed through unchanged

### Why it happens
- Current IBAN pattern expects contiguous uppercase alphanumeric format.
- Real-world IBAN input often includes spaces and lowercase letters.

### Proposed solution (without regex drift)
1. Add IBAN canonicalization before detection:
   - Remove IBAN separators (spaces).
   - Uppercase canonical form.
2. Detect on canonical view and raw view; merge/dedupe matches.
3. Add IBAN checksum validation (mod-97) before classifying as violation to protect precision.
4. Keep strict regex for canonical format; do not broadly relax pattern classes.
5. Map canonical match offsets back to original string for correct replacement.

### Acceptance criteria
- Lowercase/spaced IBAN variants are detected and sanitized.
- Invalid IBAN-like noise is rejected via checksum validation.
- Existing canonical uppercase contiguous IBAN detection remains unchanged.

## Problem 9: Obfuscated password key (`p@ssword`) bypasses detection

### Observed issue
Input variant uses obfuscated key name:
- `p@ssword=Sup3rSecret!`

### Why it happens
- Current password-key detection is literal (`password|passwd|pwd`) and misses confusable/leet substitutions.

### Proposed solution (without regex drift)
1. Canonicalize candidate key names before matching (leet/confusable folding for key identifiers only).
2. Apply existing strict password-key matcher to canonicalized key names.
3. Keep value-side thresholds strict (minimum length, entropy/charset checks).
4. Restrict this normalization to assignment-key context, not full-text replacement.

### Acceptance criteria
- `p@ssword=...` and close obfuscations are detected.
- Regular text containing similar words is not over-flagged.

## Problem 10: Password values with spaces are missed

### Observed issue
Input pattern:
- `password=\"correct horse battery staple\"`

### Why it happens
- Existing password value regex excludes spaces inside quoted values.

### Proposed solution (without regex drift)
1. Parse assignment expressions (`key = value`) with lightweight lexical rules instead of extending regex greedily.
2. If key is sensitive (`password` family), treat full quoted value as secret payload, including spaces.
3. Enforce max-length and quote-balance checks to avoid runaway captures.

### Acceptance criteria
- Quoted multi-word password values are detected and sanitized.
- Unquoted normal prose is not captured as password value.

## Problem 11: Secrets split across delimiters evade single-token regex

### Observed issue
Example split:
- `john.doe + @example.com`

### Why it happens
- Regex expects contiguous token structures (e.g., full email in one span).

### Proposed solution (without regex drift)
1. Add optional adjacency reconstruction in high-risk contexts (small delimiter gaps only).
2. Attempt recomposition for known token types (email, key fragments) with strict distance limits.
3. Mark reconstructed detections as lower-confidence (`WARN`) unless validator passes strongly.

### Acceptance criteria
- Common deliberate split patterns are detected when fragments are nearby.
- Distant or unrelated tokens are not incorrectly merged.

## Problem 12: Base64-encoded secret blobs bypass detection

### Observed issue
Input contains base64 payload that may hide credentials.

### Why it happens
- Detector matches plaintext patterns; encoded payloads are opaque.

### Proposed solution (without regex drift)
1. Add bounded decode probe for high-entropy base64 candidates:
   - Length threshold and charset gate.
   - Decode with strict size cap.
2. Run normal detectors on decoded preview.
3. Flag as `WARN` unless decoded content matches strong credential patterns.

### Acceptance criteria
- Encoded payloads containing clear credential signatures are surfaced.
- Random base64 noise does not cause high false-positive `BLOCK` decisions.

## Problem 13: Newline/escape fragmentation inside credentials causes partial misses

### Observed issue
Example:
- `postgres://admin:Very\\nSecret@db.internal:5432/app`

### Why it happens
- Connection-string matcher can break on whitespace/newline boundaries.
- Escaped transport representations may differ from runtime text form.

### Proposed solution (without regex drift)
1. Add transport unescape normalization pass (single-pass JSON escape interpretation in detection copy).
2. Normalize line-break variants for credential URI scanning.
3. Keep URI regex strict; validate scheme + authority structure post-match.

### Acceptance criteria
- Escaped/newline-fragmented credential URIs are detected.
- Non-URI multiline text is not broadly classified as credential.


### Drift-safe guard
- Maintain strict scheme-based credential pattern and enforce this baseline test in CI.

## Suggested validation matrix
1. Known-good provider keys (legacy + modern variants).
2. Long uppercase/alphanumeric payloads that are not SWIFT.
3. Near-miss strings (wrong lengths/segments) for SWIFT and API keys.
4. Mixed prompts containing both true credentials and lookalike noise.

## Rollout guidance
1. Ship regex changes behind tests first.
2. Run blind-spot Postman collection before and after.
3. Track false-positive rate and missed-detection rate from logs.
4. Promote/adjust only based on measured outcomes.
