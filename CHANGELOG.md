# Changelog

## 1.0.11

### Gateway fixes

- Separate model lookup, round-robin key selection and actual upstream attempt counts.
- Reject ambiguous model names; remove unsafe stripping of provider prefixes.
- Propagate cancellation to upstream HTTP requests and retry backoff; preserve upstream error status and `Retry-After`.
- Share authentication, prompt injection, quotas, rate limits and concurrency rules between HTTP and WebSocket messages. Temporary keys now work over WebSocket.
- Enforce per-temporary-key rate and concurrency settings. Use an atomic global concurrency reservation.
- Validate local rules before charging temporary-key request quotas. Quotas count admitted upstream requests (including failed upstream attempts), not successful completions; retries within a request do not consume another temporary quota.
- Always roll back quota transactions on early exit. Fix legacy key migration with the single SQLite connection.
- Preserve the client response format when forcing a different upstream stream mode. Convert text and tool calls between JSON and SSE, remove stream-only options from non-stream upstream requests, and restore public model aliases in responses.
- Parse SSE across packet boundaries, support `data:` with or without a space, and distinguish cancellation/truncation from success.
- Reject missing/null/malformed model lists and preserve existing models on empty discovery responses; Vertex model discovery is explicitly unsupported (manual models remain supported).
- Return empty arrays for empty model/provider lists.

### Configuration and administration

- Default connection tests select an enabled pool key and no longer send a hard-coded `max_tokens: 5`.
- Reject legacy provider-level API-key replacement with an actionable error; replace individual keys via `PUT /api/providers/:id/api-keys/:keyId` with `api_key`. Existing provider-level keys migrate to the key pool.
- Return saved proxy URLs to the edit form. Validate provider URLs, header JSON and numeric settings.
- New installations without `ADMIN_PASSWORD` generate a random initial password in startup logs. Existing passwords are preserved during upgrades; the environment variable initializes new accounts only.
- Rate-limit login attempts. Trust only loopback reverse proxies by default; set `TRUSTED_PROXIES` for a Docker proxy network.
- Compose publishes to `127.0.0.1` by default for a host Nginx reverse proxy. Set a different bind address explicitly if direct access is needed.
- `UPSTREAM_TIMEOUT_SECONDS` configures total upstream request duration (default 300, maximum 86400).
- `INCLUDE_STREAM_USAGE=false` disables automatic insertion of usage options (explicit client options are retained).
- Local token counts remain estimates; actual upstream usage takes precedence.

### Verification

Regression tests cover rotation/accounting, routing, both stream conversions, tool-call assembly, upstream errors, cancellation, malformed discovery, quota rollback, temporary limits, concurrent admission, WebSocket policy enforcement, and legacy database migration. Frontend version and backend fallback are aligned with `VERSION`.

### Compatibility notes

- Clients must use exact published model IDs or an unambiguous original ID. Invalid prefixes no longer silently route elsewhere.
- Forced non-stream upstream calls still wait for the full generation before streaming the converted result to a streaming client.
- Rate/concurrency windows are process-local and reset on restart. VTE remains a single-instance SQLite gateway.
- `/v1/responses`, native Anthropic Messages and other non-Chat-Completions protocols are not added by this release.
