# Changelog

## 1.1.0

### Frontend redesign

- New warm-neutral visual theme with an emerald accent, applied through a
  global stylesheet: Element Plus primary color and shades overridden, warm
  off-white/gray surfaces, larger card radii, softer shadows, and unified
  page headings.
- Refreshed sidebar (warm gradient, brand dot, pill-style active menu item),
  header, login page (warm emerald gradient) and statistic cards. Dark mode
  updated to match.

### Changes

- Token statistics: removed the trend line chart (and the ECharts dependency
  in the view); the page now shows the numeric overview cards and the
  per-model table only. Server time / next-reset info moved to the footer.
- Removed the Logs page, its navigation entry and route.
- Fixed a memory leak on the token-stats page (a window `resize` listener was
  never removed on unmount) and dropped some dead code.

## 1.0.12

### Security and robustness

- Limit gateway request body size to guard against memory exhaustion from oversized payloads. The default cap is 32MB (matching the WebSocket read limit) and is configurable via `MAX_REQUEST_BODY_MB`; requests exceeding it receive HTTP 413.
- Add graceful shutdown: the server now handles `SIGINT`/`SIGTERM` (e.g. `docker stop`), draining in-flight requests for up to 30 seconds before exiting instead of terminating abruptly.
- Upgrade dependencies for security and maintenance: `gin` 1.9.1 → 1.10.1, `golang.org/x/net` 0.17.0 → 0.33.0, `golang.org/x/crypto` 0.18.0 → 0.31.0, `golang-jwt/jwt/v5` 5.2.0 → 5.2.1, `gorilla/websocket` 1.5.1 → 1.5.3. Go 1.21 compatibility is retained.

### Configuration

- `MAX_REQUEST_BODY_MB` sets the maximum accepted request body size in megabytes (default 32).

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
