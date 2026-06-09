# Offline Ticket Validation / PWA MVP — Implementation Reference

## Overview

This document describes the techniques used to make the corporate event ticketing
system work offline as a Progressive Web App, covering the employee ticket-viewing
flow and the staff offline check-in flow.

No backend changes were required. All offline logic is frontend-only, using
existing APIs (`/api/v1/checkins/offline-sync`).

---

## Technology Stack

| Concern | Technology | Why |
|---------|-----------|-----|
| App install & offline shell | **Service Worker** (vanilla JS) | Cache HTML shell + hashed assets; no framework needed |
| Structured offline storage | **IndexedDB** (thin Promise wrapper) | Key-value store for tickets and check-in packages |
| Auth persistence | **localStorage** | Small payload (<1 KB), fast synchronous read at boot |
| Token hash verification | **Web Crypto API** (`crypto.subtle.digest`) | SHA-256 matching Go backend's `sha256.Sum256` |
| Unique scan IDs | **`crypto.randomUUID()`** | Cryptographically secure, no external deps |
| QR scanning | **Existing `MobileQrScanner`** component | Camera-based scanning already built |

### Intentionally Avoided

- No Workbox or PWA framework — the service worker is ~80 lines of vanilla JS.
- No `idb` library — the IndexedDB wrapper is ~95 lines.
- No HMAC secret on the frontend — offline validation uses hash membership, not
  signature verification.

---

## Architecture Components

```
┌──────────────────────────────────────────────────────────┐
│                      Browser                              │
│                                                           │
│  ┌─────────────┐  ┌──────────────┐  ┌──────────────────┐ │
│  │ Service      │  │ React SPA    │  │ IndexedDB        │ │
│  │ Worker       │  │              │  │  ├─ tickets      │ │
│  │ (sw.js)      │  │ App.tsx      │  │  └─ checkin-pkgs │ │
│  │              │  │ pages.tsx    │  │                  │ │
│  │ Cache:       │  │ offline-*.tsx│  │ localStorage     │ │
│  │  shell HTML  │  │ checkin-*   │  │  └─ auth-session │ │
│  │  /assets/*   │  │              │  │                  │ │
│  └──────┬───────┘  └──────┬───────┘  └────────┬─────────┘ │
│         │                 │                    │           │
└─────────┼─────────────────┼────────────────────┼───────────┘
          │                 │                    │
          │  Network-first  │  API calls         │  Persist
          │  for navigation │  (online only)     │  offline
          ▼                 ▼                    ▼
     ┌─────────┐     ┌───────────┐
     │ CDN /   │     │ Go API    │
     │ Origin  │     │ Server    │
     └─────────┘     └───────────┘
```

---

## Service Worker Strategy

File: `apps/web/public/sw.js`

| Request type | Strategy | Rationale |
|---|---|---|
| Navigation (`mode: 'navigate'`) | **Network-first**, fall back to cached `/index.html` | SPA always boots from the same shell |
| `/assets/*` (Vite-hashed) | **Cache-first** | Content-addressed filenames never change |
| `/api/*`, `/healthz`, `/readyz` | **Pass-through** (no interception) | API responses are managed by IDB, not SW cache |

```
sequenceDiagram
    participant B as Browser
    participant SW as Service Worker
    participant C as Cache Storage
    participant N as Network

    Note over SW: install event
    SW->>N: fetch /index.html
    SW->>C: cache.put("cets-shell-v1", /index.html)

    Note over SW: navigation request
    B->>SW: GET /user/tickets (navigate)
    SW->>N: fetch /user/tickets
    alt network available
        N-->>SW: 200 HTML
        SW-->>B: response
    else offline
        SW->>C: cache.match(/index.html)
        C-->>SW: cached shell
        SW-->>B: cached shell (SPA boots)
    end

    Note over SW: asset request
    B->>SW: GET /assets/index-Bj_lVm84.js
    SW->>C: cache.match(request)
    alt cache hit
        C-->>SW: cached asset
        SW-->>B: cached asset
    else cache miss
        SW->>N: fetch(request)
        N-->>SW: response
        SW->>C: cache.put(request, response)
        SW-->>B: response
    end
```

---

## Offline Auth Flow

Files: `auth-cache.ts`, `App.tsx`

```
sequenceDiagram
    participant U as User
    participant App as App.tsx
    participant API as /api/v1/me
    participant LS as localStorage

    Note over App: Online login (normal flow)
    App->>API: GET /me
    API-->>App: { actor, permissions }
    App->>LS: cacheAuthSession(session)
    App->>U: render authenticated shell

    Note over App: Offline reload
    U->>App: reload page (offline)
    App->>API: GET /me
    API--xApp: network error
    App->>App: navigator.onLine === false
    App->>LS: loadCachedAuthSession()
    LS-->>App: { actor, permissions }
    App->>U: render shell + "離線模式" banner
```

Key constraints:
- `loadCachedAuthSession()` validates that `actor.id` and `actor.role` exist before
  returning — corrupt data returns `null`.
- The cached session is never used when online; the API is always called first.

---

## Employee Offline Ticket Flow

Files: `tickets-store.ts`, `pages.tsx`

```
sequenceDiagram
    participant U as Employee
    participant P as TicketListPage
    participant API as /api/v1/tickets
    participant IDB as IndexedDB (tickets)

    Note over P: Online visit
    P->>API: GET /tickets
    API-->>P: ticket[]
    P->>IDB: cacheTickets(tickets)
    P->>U: render ticket list + QR codes

    Note over P: Offline revisit
    U->>P: navigate to /user/tickets
    P->>API: GET /tickets
    API--xP: network error
    P->>P: isOffline() === true
    P->>IDB: loadCachedTickets()
    IDB-->>P: cached ticket[]
    P->>U: render cached tickets + "離線模式" banner
```

The QR display works from the cached `signed_token` field — no additional offline
handling was needed for QR rendering.

---

## Staff Offline Check-in Flow

Files: `checkin-store.ts`, `checkin-logic.ts`, `offline-package-step.tsx`,
`offline-scan-step.tsx`, `use-offline-sync.ts`, `offline-page.tsx`

### Phase 1: Package Download

```
sequenceDiagram
    participant S as Staff
    participant UI as OfflinePackageStep
    participant API as /api/v1/checkins/offline-package
    participant IDB as IndexedDB (checkin-packages)

    S->>UI: select event + device, click "下載離線名單"
    UI->>API: GET /offline-package?event_id=...&device_id=...
    API-->>UI: { batch_id, valid_until, tickets[], ticket_count, package_signature }
    UI->>IDB: savePackage(pkg, staffID)
    UI->>S: show batch info, switch to scan tab
```

### Phase 2: Offline Scanning

```
sequenceDiagram
    participant S as Staff
    participant UI as OfflineScanStep
    participant Logic as checkin-logic.ts
    participant Hash as hash.ts (Web Crypto)
    participant IDB as IndexedDB

    S->>UI: scan QR code (camera or manual input)
    UI->>UI: token = scanned text (trimmed)

    UI->>Logic: judgeOfflineScan(token, pkg, existingScans)
    Logic->>Logic: check empty token → conflict
    Logic->>Logic: check isPackageExpired → conflict
    Logic->>Hash: hashToken(token) → SHA-256 hex
    Hash-->>Logic: tokenHash
    Logic->>Logic: find tokenHash in pkg.tickets
    alt not found
        Logic-->>UI: { status: "conflict", reason: "not_in_offline_package" }
    else found
        Logic->>Logic: check tokenHash in existingScans
        alt already scanned
            Logic-->>UI: { status: "duplicate" }
        else first scan
            Logic-->>UI: { status: "accepted", matched_ticket }
        end
    end

    UI->>IDB: addScanRecord(batchID, record)
    Note over UI: record.scanned_at = new Date().toISOString()
    UI->>S: show result badge + update KPI counts
```

### Phase 3: Sync on Reconnect

```
sequenceDiagram
    participant S as Staff
    participant Hook as useOfflineSync
    participant IDB as IndexedDB
    participant Logic as checkin-logic.ts
    participant API as /api/v1/checkins/offline-sync

    Note over Hook: 'online' event or manual "立即同步" click
    Hook->>IDB: markScansAsSyncing(batchID)
    Hook->>IDB: loadPackage(batchID)
    IDB-->>Hook: stored (with scans[])

    Hook->>Logic: buildSyncPayload(stored)
    Note over Logic: Each scan keeps its own scanned_at timestamp
    Logic-->>Hook: { batch_id, device_id, scans[] }

    Hook->>API: POST /offline-sync
    API-->>Hook: { results[] }

    Hook->>IDB: updateScansFromSync(batchID, results)
    Note over IDB: Match by token_hash, update server_status, server_checkin_id
    Hook->>IDB: markBatchSynced(batchID)
    Hook->>S: show sync results, disable further scans
```

---

## IndexedDB Schema

Database: `cets-offline`, version 1

| Store | Key path | Contents |
|-------|----------|----------|
| `tickets` | `ticket_id` | Full ticket objects from employee list API |
| `checkin-packages` | `batch_id` | `StoredCheckinPackage` with nested `scans[]` |

The wrapper (`db.ts`) exposes: `openDB`, `putItem`, `getItem`, `getAllItems`,
`deleteItem`, `clearStore`. Each wraps IDBRequest in a Promise. A module-level
reference caches the database connection after first open.

---

## Token Hash Matching

The offline check-in package contains `token_hash` (SHA-256 hex) for each ticket.
The frontend hashes scanned tokens with Web Crypto to check membership locally:

```typescript
// frontend (hash.ts)
const digest = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(token));
return Array.from(new Uint8Array(digest)).map(b => b.toString(16).padStart(2, "0")).join("");
```

This matches the Go backend's:
```go
hash := sha256.Sum256([]byte(token))
hex.EncodeToString(hash[:])
```

The raw signed token is never displayed in the UI — only the hash is used for
local logic, and the token is sent to the server only during sync.

---

## Per-Scan Timestamps (Bug Fix)

Previous behavior: all scans in a batch shared a single `scanned_at` timestamp
generated at sync time.

New behavior: each `OfflineScanRecord` captures `scanned_at: new Date().toISOString()`
at the moment the QR code is scanned. `buildSyncPayload()` preserves these per-scan
timestamps when building the sync request.

---

## Batch Lifecycle

```
active ──(sync success)──► synced
  │                           │
  │  can add scans            │  no new scans allowed
  │  can sync                 │  staff must download new package
  └───────────────────────────┘
```

After a successful sync, the batch is marked `"synced"` and the scanner is disabled.
Staff must download a new package for further scanning.
