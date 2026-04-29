# Auth Failure Triage

Pick a symptom and walk the recovery path

## Walkthrough

### How this demo works

You'll be asked to pick a failure symptom. Each branch shows the
recovery flow for that case. At the end you can loop back and
try a different one.

### 1. Pick a symptom

Most auth failures fall into a handful of buckets.

**Inputs:**

- `symptom` = `expired`

```
Investigating: expired
```

→ jumped to `expired`

### 2. Expired token

The access token's TTL has elapsed; the API rejects it.

```
API said: 401 token_expired
```

> **Error:** token expired

### 3. Refresh now?

**Inputs:**

- `retry` = `yes`

→ jumped to `refresh`

### 4. Refresh succeeds

```
New token: eyJhbGci...truncated
```

→ jumped to `recovered`

### 5. Recovered

Application has a usable token again.

```
Resumed normal operation.
```

→ jumped to `loop`

### 6. Try another symptom?

**Inputs:**

- `again` = `no`

### 7. Abandoned

> **Info:** Skipped recovery; user gives up.

### 8. End

