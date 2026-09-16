---
title: Gracefully finish OAuth callback responses before closing the server
status: open
priority: high
area: auth
reported: 2026-07-29
---

# Gracefully finish OAuth callback responses before closing the server

## Summary

`agora login` can close its local OAuth callback server before the browser has
received the rendered success or error page. This causes intermittent browser
request failures even when token exchange and session persistence succeed.

## Evidence

The Ubuntu CI run for `TestCLILoginAndWhoAmI` failed with:

```text
expected localized CN success page, got status=0 body=
```

The same test passed on a subsequent run without a code change, indicating a
timing-dependent failure.

## Root cause

`callbackServer.CompleteSuccess` unblocks after the callback handler writes the
response body, but before the handler returns and the HTTP response is fully
flushed to the client. `login` then returns and its deferred
`callbackServer.Close` calls `http.Server.Close`, which can terminate the
active callback connection.

The integration test records the browser response only when `http.DefaultClient.Do`
succeeds. On a connection error it leaves the status at `0` and the body empty,
which obscures the underlying transport error.

## Impact

- A user can complete OAuth authentication successfully but see a blank or
  failed browser callback page.
- Login integration tests can fail intermittently, especially on Linux CI.

## Proposed fix

1. Replace immediate callback-server shutdown with graceful shutdown that waits
   for the callback handler to return before closing active connections.
2. Bound graceful shutdown with a context timeout so login cannot hang during
   cleanup.
3. Keep listener cleanup idempotent for the IPv4 and IPv6 callback listeners.
4. Update `TestCLILoginAndWhoAmI` to capture and report the browser request
   error instead of reporting only `status=0 body=`.
5. Add a regression test that verifies the client receives the complete success
   page before callback-server shutdown completes.

## Acceptance criteria

- The browser receives HTTP 200 and the complete localized success page after a
  successful login.
- Error paths still return their intended status and safe error page.
- `go test -count=1 ./...` is stable across repeated Linux runs.
- Callback shutdown has a bounded timeout and does not leak listeners or
  goroutines.

