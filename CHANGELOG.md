# Changelog

## 1.2.0

### Gateway

- **Key rotation**: when the upstream rejects a key with 401, 403 or 429, the
  request is retried immediately with the next active key of the provider.
  Rotation does not count towards "max retries"; when every key has been
  tried the last upstream error is returned.
- **Long streams no longer cut off**: `UPSTREAM_TIMEOUT_SECONDS` is now the
  maximum upstream *silence* — the wait for response headers and the gap
  between two chunks of the body — instead of an `http.Client.Timeout` on the
  whole exchange, which cut streams longer than 5 minutes. A stream that
  stalls for longer than the timeout is still aborted.
- **No duplicate upstream calls**: requests are retried after network errors
  only if they never reached the upstream (connection, DNS or proxy
  failure). Timeouts and errors after the request was sent are returned
  immediately instead of being retried up to four times.
- Token statistics use the model display name for both streaming and
  non-streaming requests (previously a model requested by its original ID
  was recorded under two names).
- Settings used by the gateway are read with one query per request instead
  of about ten.
- WebSocket: the API key can be passed as a subprotocol
  (`new WebSocket(url, ["bearer", key])`). `?api_key=` still works.

### Models and providers

- Fetch Models no longer deletes manually added models, and only disables
  (instead of deleting) renamed models and models created before 1.2.0 that
  are missing from the upstream list. New `models.source` column.
- Deleting a provider also deletes its API keys, in one transaction. Keys
  and models orphaned by earlier versions are cleaned up on startup.

### Accounts and security

- Login tokens carry the user ID: changing the username no longer logs you
  out. Tokens issued by earlier versions keep working.
- Changing the password revokes every token issued before the change
  (other devices must log in again); the current session receives a new
  token. New `users.password_changed_at` column.
- `/api/version/check` requires login and caches the GitHub result for an
  hour (5 minutes after a failure).
- `crypto/rand` failures no longer fall back to predictable API keys or JWT
  secrets.

### Operations

- Token estimation uses embedded BPE files (`tiktoken-go-loader`) instead of
  downloading them from `openaipublic.blob.core.windows.net` at runtime.
- The token-stats reset is configurable with `TOKEN_STATS_TZ` and
  `TOKEN_STATS_RESET_HOUR` (default Asia/Shanghai 15:00). Time zone names
  follow daylight saving time.
- SQLite pragmas are set in the connection string so they survive
  reconnects; write errors in model handlers are reported instead of
  ignored.
- Removed the unused `/api/logs` endpoints and the `hourly_stats` field.
- The Docker build injects the version from `VERSION`.

### Model names

- Display names must be unique: renaming, resetting a name, adding a model
  and changing a provider prefix are rejected when they would duplicate
  another model's name, and enabling a model is rejected when another
  enabled model has the same name (previously requests then failed with
  "ambiguous model"). Fetch Models still adds duplicates (disabled) and
  reports how many need a prefix or alias.
- Renaming a model, resetting its name or changing a provider prefix now
  updates every reference in the same transaction: temporary API keys
  (allowed models, per-model limits and usage) and custom rate-limit rules.
  Previously those keys and rules silently stopped matching.
- Models disabled by Fetch Models because they disappeared upstream are
  re-enabled automatically when they reappear (`models.disabled_by_sync`);
  models you disabled yourself stay disabled.
- Listing models no longer rewrites every display name on each request.
- Model lists include `source` and `disabled_by_sync`; the UI tags manual
  models and models that went offline upstream.

### Providers and accounts

- Extra request headers (`extra_headers`) can be edited in the provider
  dialog and are returned by the provider list.
- New passwords must be at least 8 characters.

### Deployment

- Docker images build with Go 1.24 and Node 22 (previously Go 1.21 and
  Node 18, both end-of-life), matching CI.
- `TRUSTED_PROXIES` is documented, with the setting needed when VTE runs in
  Docker behind a host reverse proxy.

### Frontend: settings

- Settings has an "访问限制" section again for global rate limit,
  global concurrency limit and custom per-provider/per-model rules.
- Token stats show the configured reset time and time zone.
- On phones the header shows the current page title; browser tab titles
  follow the page.

### Frontend fixes

- Copy buttons now work when the panel is opened over plain HTTP
  (e.g. `http://IP:8050`): falls back to `execCommand('copy')` when the
  Clipboard API is unavailable, and reports failure instead of always
  showing "copied".
- Cancelling a confirmation dialog no longer raises an unhandled promise
  rejection.
- Errors are shown once: the axios interceptor is the single place that
  displays request errors; views no longer add a second toast.
- 401 handling goes through the router and the user store (no full page
  reload, one "session expired" message); a wrong password on the login
  page shows the error instead of reloading the page. Transient errors when
  loading the current user no longer log the user out.
- Switches (model / key enable) roll back when the request fails.
- Models: status filter works again after clearing it; page resets to 1 when
  filters change; removed a no-op `onActivated` hook.
- Settings: username field is filled once the user profile loads; theme
  radio stays in sync with the header toggle; each section has its own
  saving state; new password must be entered twice.
- About: dark-mode colours fixed (code blocks were light-on-light); opening
  the page no longer pops a "latest version" toast.
- Token stats: background polling pauses while the tab is hidden and no
  longer shows an error toast every 10 seconds when offline.
- Theme is applied on the login page too.

### Mobile

- Shared `useIsMobile()` composable (reactive to resizing/rotation) replaces
  three separate checks.
- Providers and temporary API keys render as cards on narrow screens; the
  provider action column is reduced to "Models / Test / More".
- Dialogs and forms use top-aligned labels on phones; temp-key dialog is
  fullscreen; tables hide secondary columns.

### Cleanup

- Removed unused `echarts` and `dayjs` dependencies (left over from the
  removed trend chart).
- Icons are imported per component instead of registering every Element
  Plus icon globally.
- New `utils/` helpers (`copyText`, `confirmAction`, `formatDateTime`,
  `formatNumber`) and an `ApiKeyField` component shared by Dashboard and
  Settings.
- README no longer lists the removed real-time logs page.

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
