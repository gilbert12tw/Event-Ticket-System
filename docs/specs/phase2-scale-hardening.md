# Phase 2 Scale Hardening — Acceptance Matrix and Non-Goals

> Canonical scope doc for Linear project `Phase 2 Scale Hardening` (`COR-39`～`COR-83`)。
> Phase 1 baseline 驗收依據：`docs/specs/phase1-production-upper-bound.md`。
> Phase Roadmap 演進原則：`docs/ARCHITECTURE.md` §3。
> NFR / 容量總表：`docs/specs/phase1-nfr-and-capacity.md` §5。

本文件作為 Phase 2 acceptance matrix + non-goals 的 single source of truth。Child issue (`COR-44`～`COR-83`) 必須對齊本文件的 goals、non-goals、workstream ownership 與 cross-stream dependency。Phase 2 的主軸是 scale hardening，不是 Phase 1 → 完整微服務的一次性改寫。預設仍是 Phase 1 modular monolith；只有在量測證據支持時，才做 process-level scaling 或更重的架構演進。

---

## 1. Phase 2 Goals — Capacity / Correctness / Latency / Ops

### 1.1 容量目標

| 維度 | 目標 |
| --- | --- |
| 員工規模 | 50,000 |
| 熱門活動同時段 | 一個 limited hot event |
| App RPS | 270 |
| Booking TPS | 35 |
| Active VUs | 2,400 |

### 1.2 正確性目標

- Confirmed bookings 不可超過 event capacity。
- 同一組 `(event_id, employee_id)` 不可有重複 confirmed booking。
- Idempotent retry 必須回傳相同 booking result。

### 1.3 延遲目標

| 操作 | p95 | p99 |
| --- | ---: | ---: |
| browse / detail | < 500ms | < 1000ms |
| booking HTTP | < 750ms | < 1500ms |
| booking transaction (server-side) | — | < 500ms |

### 1.4 營運與可靠性目標

| 指標 | 目標 |
| --- | --- |
| outbox lag p95 | < 60s |
| outbox lag max | < 180s |
| reporting freshness p95 | < 60s |
| unexpected 5xx | < 0.5% |

---

## 2. Non-Goals — Deferred Decision-Gate Topics

Phase 2 明確 **不** 交付下列項目；若 implementation issue 出現相關語言，視為 scope drift，必須先升級為新 spec / decision gate 才能執行：

- 不預設把 Registration 拆成獨立部署服務。
- **Kafka、Kubernetes、service mesh、完整微服務、cross-region HA 不列為 Phase 2 必交付範圍**；這些屬於 deferred decision-gate topics，留待 Phase 3 或新 spec 評估。
- 不把 Redis reservation success 視為 booking success。
- 不用 reporting read model 當作 booking、eligibility、ticket redemption 或 audit 的真相來源。
- 不把 Phase 1 尚未完成的 backlog 吸收到 Phase 2 issue 裡 (只能 reference)。
- 除非 decision gate 證明 check-in 是 Phase 2 瓶頸，否則不實作 check-in cache。

> Docs guard `TestPhase2DocsDoNotClaimDeferredInfraIsRequired` (`services/api/internal/architecture/architecture_test.go`) 會在 Phase 2 docs 出現「將 Kafka / Kubernetes / service mesh / cross-region HA 列為 Phase 2 必交付」的語言時失敗。Forbidden phrase list 見該 test。

---

## 3. Workstream Ownership Matrix

| 人員 | 主責 Workstream | Linear Parent | 定位 |
| --- | --- | --- | --- |
| Person A | WS1 Contracts + Release Engineering | `COR-39` / `[PH2-WS1]` | Phase 2 acceptance matrix、spec、API/event contract、docs guardrail、release checklist、reviewer checklist。 |
| Person B | WS2 Load + Observability | `COR-40` / `[PH2-WS2]` | Load seed、hot-event k6 profile、server-side metrics、trace/log、baseline report、performance CI gate。 |
| Person C | WS3 Registration Hot Path | `COR-41` / `[PH2-WS3]` | Booking hot path、Redis pre-admission gate、booking idempotency、contention reduction、capacity pressure API、oversell failure tests。 |
| Person D | WS4 Async Platform + Notification Isolation | `COR-42` / `[PH2-WS4]` | Outbox envelope、same-binary worker kind split、notification isolation、retry/backoff/dead-letter、replay process、worker recovery。 |
| Person E | WS5 Reporting + Ops Control Plane | `COR-43` / `[PH2-WS5]` | Reporting read model spec、projection schema、read model worker、rebuild process、report freshness contract、exports、ops UI、check-in cache decision gate。 |

每個 child issue 屬於 **exactly one** primary workstream。Cross-stream review 由 owner 自行 ping 對應 reviewer，不視為改變 ownership。

### 3.1 WS1: Contracts + Release Engineering

Issues：

- `COR-44` / `[PH2-00] Phase 2 acceptance matrix and non-goals` (本文件)
- `COR-45` / `[PH2-01] Phase 2 specs split by workstream`
- `COR-46` / `[PH2-02] OpenAPI delta for ops and report freshness`
- `COR-47` / `[PH2-03] Domain event contract v2`
- `COR-48` / `[PH2-04] Architecture docs update for process-first Phase 2`
- `COR-49` / `[PH2-05] Contract drift and docs guard tests`
- `COR-50` / `[PH2-06] Phase 2 release checklist`
- `COR-51` / `[PH2-07] Linear reviewer checklist template`

Owner responsibilities：

- 在 implementation 開始前定義 Phase 2 共用規則。
- 維持 Phase 2 process-first、evidence-driven。
- 把 non-goals 寫清楚，避免 implementation issue 漂移成 Kafka、Kubernetes 或過早微服務拆分。
- 確保每個 stream 都有可測試的 acceptance criteria、edge cases、non-functional requirements、12-Factor notes、non-goals。

When to start：Wave 0 / Wave 1。WS1 第一波 contract 完成後，Person A 應把時間投到 WS3/WS4/WS5 review，不要持續新增 planning 文件。

### 3.2 WS2: Load + Observability

Issues：

- `COR-52` / `[PH2-10] 50k employee load seed command`
- `COR-53` / `[PH2-11] Single-hot-event k6 profile`
- `COR-54` / `[PH2-12] Phase 2 k6 threshold gate`
- `COR-55` / `[PH2-13] DB lock and pool wait metrics`
- `COR-56` / `[PH2-14] Queue lag and worker metrics`
- `COR-57` / `[PH2-15] Trace/log schema for Phase 2 hops`
- `COR-58` / `[PH2-16] Capacity baseline report`
- `COR-59` / `[PH2-17] Performance regression CI wiring`

Owner responsibilities：

- 建立可重現的 baseline，證明瓶頸在哪裡。
- 確保 hot-event profile 打 single limited event，而不是很多互不相干 events。
- 分開量測 HTTP latency 與 server-side booking transaction timing。
- 產出可以支持決策的 baseline report，不只是 pass/fail gate。

When to start：Wave 1 起跑，不阻擋 WS3/WS4/WS5 spec work。`PH2-16` 必須回答瓶頸是 registration tx time / DB lock wait / DB pool wait / browse-detail latency / outbox lag / worker throughput / reporting aggregation / read model freshness 哪一段。

### 3.3 WS3: Registration Hot Path

Issues：

- `COR-60` / `[PH2-20] Redis reservation gate spec`
- `COR-61` / `[PH2-21] Event/actor rate limit middleware`
- `COR-62` / `[PH2-22] Redis Lua pre-admission reservation`
- `COR-63` / `[PH2-23] Reservation TTL and compensation`
- `COR-64` / `[PH2-24] Booking idempotency result hardening`
- `COR-65` / `[PH2-25] Hot-row contention reduction`
- `COR-66` / `[PH2-26] Capacity pressure admin API`
- `COR-67` / `[PH2-27] Redis failure and oversell regression tests`

Owner responsibilities：

- 保留 PostgreSQL 作為 booking final truth。
- Redis 只做 pre-admission，不做 committed booking truth。
- 在依賴 Redis、TTL compensation 或 contention changes 前，先強化 booking idempotency。
- 用測試證明 Redis failure / retry / timeout / DB rollback 情境下仍不會 oversell 或 duplicate booking。

When to start：Spec (`PH2-20`, `PH2-24`) 可在 Wave 1 起跑；hot-path implementation 等 `PH2-20`、`PH2-24`、`PH2-16` 完成。

### 3.4 WS4: Async Platform + Notification Isolation

Issues：

- `COR-68` / `[PH2-30] Outbox envelope v2 migration`
- `COR-69` / `[PH2-31] Worker kind split config`
- `COR-70` / `[PH2-32] Notification worker isolation`
- `COR-71` / `[PH2-33] Retry, backoff, and dead-letter policy`
- `COR-72` / `[PH2-34] Queue replay admin process`
- `COR-73` / `[PH2-35] Worker graceful shutdown hardening`
- `COR-74` / `[PH2-36] Notification delivery admin visibility`
- `COR-75` / `[PH2-37] Worker crash/retry integration tests`

Owner responsibilities：

- Worker scaling 只做 same-binary process types，不做獨立服務拆分。
- 使用 PostgreSQL transactional outbox 作為可靠 async boundary。
- 在 replay、retry、dead-letter、projection 依賴前，先定義 consumer idempotency。
- 確保 worker shutdown 與 crash recovery 不會造成 side effects 重複。

When to start：Wave 1 起跑，不需等 WS3。Runtime optimization 仍要依 WS2 baseline evidence。

### 3.5 WS5: Reporting + Ops Control Plane

Issues：

- `COR-76` / `[PH2-40] Reporting read model spec`
- `COR-77` / `[PH2-41] Reporting projection schema`
- `COR-78` / `[PH2-42] Read model worker`
- `COR-79` / `[PH2-43] Read model rebuild admin process`
- `COR-80` / `[PH2-44] Reports API freshness contract`
- `COR-81` / `[PH2-45] Export reads from reporting projection`
- `COR-82` / `[PH2-46] Ops UI for capacity, queues, and report lag`
- `COR-83` / `[PH2-47] Conditional check-in cache decision gate`

Owner responsibilities：

- Reporting projections 必須是 derived、disposable、rebuildable、non-authoritative。
- 防止 read model 被用於 booking、eligibility、check-in、ticket redemption 或 audit truth。
- 在 API 與 UI 中明確揭露 report freshness。
- 除非 check-in-specific evidence 證明 check-in 是 Phase 2 bottleneck，否則 check-in cache 只保留為 decision gate。

When to start：`PH2-40` / `PH2-41` spec 可在 Wave 1 起跑；read model worker 等 outbox envelope 與 worker kind contracts 穩定。

---

## 4. Cross-Stream Dependency Map

下表記錄 child issue 的關鍵 input gating，用以 reviewer 判斷 issue 是否可進入 implementation。本表是 §3 owner responsibilities 的橫向視圖，不引入新 scope。

| Child issue(s) | Belongs to | Depends on | Reason |
| --- | --- | --- | --- |
| `PH2-22`, `PH2-23`, `PH2-25`, `PH2-27` | WS3 | `PH2-20`, `PH2-24`, `PH2-16` | Hot-path 動 Redis / contention 前要先有 reservation spec、idempotency hardening、baseline evidence。 |
| `PH2-32`～`PH2-37` | WS4 | `PH2-03`, `PH2-30`, `PH2-31` | Worker 行為要等 event contract v2 + outbox envelope v2 + worker kind config。 |
| `PH2-42`～`PH2-45` | WS5 | `PH2-30`, `PH2-31`, `PH2-40`, `PH2-41` | Read model worker / freshness contract / export 需 outbox envelope + read model spec + projection schema。 |
| `PH2-46` | WS5 | `PH2-02`, `PH2-13`, `PH2-14`, `PH2-15`, `PH2-26`, `PH2-44` | Ops UI 需 OpenAPI delta、DB / queue metrics、trace schema、capacity pressure API、freshness contract。 |
| `PH2-47` | WS5 | `PH2-16` + check-in-specific latency / contention evidence | Decision gate；不在 Phase 2 baseline scope 內，需 evidence 才能升級成 implementation issue。 |

---

## 5. Execution Waves

### Wave 0 — Kickoff and Hygiene

目標：所有 owner 在開始 implementation 前先對齊共用規則。

必要動作：

- Person A 與所有 owner review `PH2-00`、`PH2-01`、`PH2-02`、`PH2-03`。
- Person B 與所有 owner 確認 load model 與 target metrics。
- 每位 owner 確認自己的 issues 都包含 Summary、Acceptance Criteria、Tests、12-Factor notes、Non-goals。
- 所有人同意 Phase 1 blockers 只能 reference，不得吸收到 Phase 2 completion criteria。

### Wave 1 — Parallel Enabling Work

目標：讓五人一開始都有事做，不需等所有 contracts 完成。

| 人員 | 優先開始 | 原因 |
| --- | --- | --- |
| Person A | `PH2-00`, `PH2-01`, `PH2-02`, `PH2-03` | 定義其他 streams 依賴的 acceptance matrix、OpenAPI delta、event contract。 |
| Person B | `PH2-10`, `PH2-11`, `PH2-13`, `PH2-14`, `PH2-15` | 建立 optimization 需要的 evidence base 與 observability。 |
| Person C | `PH2-20`, `PH2-24` | Redis gate rules 與 idempotency 必須先定義，才能安全改 hot path。 |
| Person D | `PH2-30`, `PH2-31` | Outbox envelope 與 worker kind config 會 unblock async 與 read model work。 |
| Person E | `PH2-40`, `PH2-41` | Read model spec 與 schema 可以先做，不需等 worker contract 全部完成。 |

### Wave 2 — Evidence-Gated Implementation

目標：runtime behavior 必須在 contracts 與 baseline evidence 到位後再實作。

| 人員 | 接續工作 | 必要輸入 |
| --- | --- | --- |
| Person C | `PH2-22`, `PH2-23`, `PH2-25`, `PH2-27` | `PH2-20`, `PH2-24`, `PH2-16` |
| Person D | `PH2-32`, `PH2-33`, `PH2-34`, `PH2-35`, `PH2-36`, `PH2-37` | `PH2-03`, `PH2-30`, `PH2-31` |
| Person E | `PH2-42`, `PH2-43`, `PH2-44`, `PH2-45` | `PH2-30`, `PH2-31`, `PH2-40`, `PH2-41` |
| Person E | `PH2-46` | `PH2-02`, `PH2-13`, `PH2-14`, `PH2-15`, `PH2-26`, `PH2-44` |
| Person E | `PH2-47` | `PH2-16` + check-in-specific latency / contention evidence |

### Wave 3 — Release Hardening

目標：證明 Phase 2 可以交付，且沒有隱藏 correctness 或 operations gap。

必要動作：

- Person A 完成 `PH2-05`、`PH2-06`、`PH2-07`。
- Person B 完成 `PH2-12`、`PH2-16`、`PH2-17`。
- Person C 證明 oversell、duplicate booking、Redis outage、Redis timeout、TTL expiry、DB rollback、idempotent retry cases。
- Person D 證明 duplicate delivery、replay、dead-letter、worker crash、graceful shutdown cases。
- Person E 證明 projection rebuild、stale read model、export privacy、report freshness、ops UI redaction cases。

---

## 6. Acceptance Criteria for `[PH2-00]`

| AC | 條件 |
| --- | --- |
| AC-1 | Given Phase 2 planning is reviewed, When the matrix is read, Then goals (§1)、non-goals (§2)、capacity targets (§1.1) 與 workstream boundaries (§3) 皆 unambiguous。 |
| AC-2 | Given a child issue starts, When its scope 與 §3 + §4 比對, Then it maps to exactly one primary workstream。 |
| AC-3 | Given reviewers inspect Phase 2 docs, When Kafka / Kubernetes / service mesh / cross-region HA / 完整微服務 出現, Then 一律 marked as deferred decision-gate topics (§2 + docs guard test)。 |

12-Factor notes：本文件不改變 codebase / process / config / build-release-run / disposability / dev-prod parity 等任何 factor；Phase 2 預設仍為 “one codebase, process-first evolution”。

Non-goals (for this issue)：

- 不實作任何 runtime behavior、不動 OpenAPI、worker code、frontend、CI workflow。
- 不寫 `PH2-01`～`PH2-47` child spec 內容。
- 不更新 `docs/ARCHITECTURE.md` §3 Phase Roadmap (留給 `PH2-04`)。

---

## 7. Verification

- `git diff --check` (docs-only change)。
- `cd services/api && go test ./internal/architecture -run TestPhase2DocsDoNotClaim -count=1` — 新 docs guard 通過。
- `cd services/api && go test ./internal/architecture -count=1` — Phase 1 guard 與 file-size guard 不退化。
- `ruby scripts/check-openapi-contract.rb` — sanity，確認沒誤動 OpenAPI contract。
- 人工 cross-check：每個 child issue (`PH2-00`～`PH2-47`) 在 §3 至少出現一次；每個 §2 non-goal 對應到 Linear issue acceptance criteria。

---

## 8. References

- Phase 1 production 驗收矩陣：`docs/specs/phase1-production-upper-bound.md`
- Phase 1 NFR & 容量：`docs/specs/phase1-nfr-and-capacity.md`
- Phase Roadmap：`docs/ARCHITECTURE.md` §3
- Architecture rules：`docs/agent-rules/architecture.md`
- Docs guard 實作：`services/api/internal/architecture/architecture_test.go`
- Linear project：`Phase 2 Scale Hardening`，parents `COR-39`～`COR-43`，本 issue `COR-44` / `[PH2-00]`。
