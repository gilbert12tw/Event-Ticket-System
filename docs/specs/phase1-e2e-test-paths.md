# Phase 1 E2E 測試路徑

## 摘要

本文件定義 Corporate Event Ticketing System 的 Phase 1 production E2E
testcase 路徑。它補充 `docs/specs/phase1-production-upper-bound.md`、
`docs/specs/phase1-product-requirements.md` 與 `docs/specs/phase1-mvp.md`，
把必要角色旅程與正確性風險整理成 Given / When / Then 場景。

範圍僅限 Phase 1 production：Go modular monolith、可選的 same-binary
worker、PostgreSQL 作為最終事實來源，以及 Docker Compose backing
services 作為本地驗證環境。本文件不得被解讀為 microservices、Kafka、
Kubernetes、service mesh、cross-region high availability、managed
failover 或其他 Phase 2/3 deferred infrastructure 已完成或為 Phase 1 必要條件。

## 驗證入口

| 層級 | 入口 | 用途 |
|---|---|---|
| Live browser E2E | `apps/web/e2e-live/phase1-live-flow.spec.ts` | 以 Compose-backed app 驗證員工、驗票、HR、通知、稽核流程，不 mock API route。 |
| Mocked browser E2E | `apps/web/e2e/core-role-routes.spec.ts` | 驗證角色路由、禁止進入狀態、版面 containment 與聚焦 UI 狀態。 |
| Frontend gate | `pnpm --filter cets-web test:e2e` | 在 375px、768px、1024px、1440px Chromium viewport matrix 驗證。 |
| Backend gate | `go test ./services/api/... -count=1` | 驗證 handler、service、worker、architecture 與 unit coverage。 |
| DB integration gate | `TEST_DATABASE_URL=... go test ./services/api/... -count=1` | 驗證 PostgreSQL-backed correctness、idempotency 與 transaction behavior。 |
| k6 smoke / release | `k6/phase1-production-gate.js`, `k6/phase1-release-gate.js` | 驗證 health、browse、book/cancel、ticket、check-in、offline sync、report/export、audit 與 release thresholds。 |

## 測試資料基線

| Profile | Role | 必要屬性 | 用途 |
|---|---|---|---|
| `employee-eligible` | employee | Active employee，department/site/grade 符合活動規則，沒有 cooldown | 成功報名、票券顯示、取消、通知。 |
| `employee-second` | employee | 另一位符合相同活動資格的 active employee | 最後席次併發、候補、重複驗票隔離。 |
| `employee-ineligible` | employee | Active employee，但 department/site/grade 不符合活動規則 | 資格拒絕與無 side effect 驗證。 |
| `employee-cross-city` | employee | 符合資格，但員工城市不同於活動城市 | 非阻斷式跨城市警示。 |
| `employee-cooldown` | employee | Active employee，仍在 no-show cooldown 期間 | Limited-event booking 拒絕。 |
| `activity-admin` | activity admin | 可治理活動、資格、報名、抽籤、票券 | 活動設定與敏感管理操作。 |
| `checkin-staff` | check-in staff | 可核銷票券與同步離線掃描，不可治理活動 | 線上與離線驗票。 |
| `hr-admin` | HR/system admin | 可查看報表、匯出與稽核紀錄 | 彙總報表與 PII redaction。 |
| `unauthorized` | none or wrong role | 缺失、過期、不完整、被竄改或角色不符的 provider claims | 401/403 與無 business mutation 驗證。 |

## Admin Event Setup

| ID | Role | Risk | Given | When | Then | Evidence | AC |
|---|---|---|---|---|---|---|---|
| P1-E2E-ADM-001 | activity admin | 管理員不能不靠 DB 手動改資料完成活動治理 | Given activity admin 具備有效 provider claims | When 他建立一個 published limited event，包含 city、site、capacity、booking window、family rule 與 eligibility rule | Then event 會以版本、可見狀態與 audit entry 持久化 | Admin event response、event version row、audit row、live admin route | AC-1, AC-14 |
| P1-E2E-ADM-002 | activity admin | Unlimited capacity 被誤當成 limited inventory | Given activity admin 建立 unlimited event | When event 被 publish | Then 後續 booking 不會扣有限庫存，UI 也標示為 unlimited | Event detail API、employee event page、DB registration rows | AC-1, AC-5 |
| P1-E2E-ADM-003 | activity admin | Zero-match eligibility 讓活動發布後無人可報名 | Given eligibility preview 回傳 0 位符合員工 | When admin 沒有 explicit confirmation 就保存規則 | Then save 被拒絕，不啟用新 rule version，admin 看得到 zero-match reason | Eligibility preview response、無 active rule version、rejected audit 或無成功 audit | AC-2 |
| P1-E2E-ADM-004 | activity admin | 非法 event state transition 破壞活動生命週期 | Given event 目前狀態不能直接轉到目標狀態 | When admin 送出 state change | Then API 回 `409`，event 不變更，也不建立新 event version | State response、event version count unchanged、audit query | AC-1 |
| P1-E2E-ADM-005 | employee | Role check 只做在 UI，API 仍可越權 | Given employee 開啟 admin event route 或呼叫 admin event API | When protected surface 載入或 request 送出 | Then 回 `401` 或 `403`，且不產生 event、rule、business audit mutation；只允許必要的 security audit evidence | Forbidden route UI、API response、DB mutation check | AC-13, AC-14 |

## Employee Booking

| ID | Role | Risk | Given | When | Then | Evidence | AC |
|---|---|---|---|---|---|---|---|
| P1-E2E-BKG-001 | employee | 員工核心旅程斷裂 | Given `employee-eligible` 符合 published limited event 且仍有剩餘席次 | When 他瀏覽 detail 並帶 idempotency key 報名 | Then UI 顯示資格、剩餘席次、confirmed registration 與 signed non-transferable ticket handoff | Live flow、registration row、ticket row、outbox/audit rows | AC-4, AC-5, AC-8 |
| P1-E2E-BKG-002 | employee | Cached eligibility 被拿來授權 final booking | Given `employee-ineligible` 仍能看到 event summary | When 他嘗試 final booking | Then booking 在 transaction 內重查 eligibility，回拒絕原因，且不建立 registration 或 ticket | Booking API response、registration/ticket row count unchanged | AC-4, AC-5 |
| P1-E2E-BKG-003 | employee | Retry 造成重複 side effects | Given 同一 idempotency key 已建立 confirmed booking | When 相同 request 再送一次 | Then 回傳原 registration 與 ticket，不新增 registration、ticket、notification 或 audit side effect | Idempotency response、DB row counts、delivery count | AC-5, AC-10 |
| P1-E2E-BKG-004 | employee | 換 idempotency key 繞過一人一票限制 | Given employee 已有同活動 active registration | When 他用不同 idempotency key 再次報名 | Then 系統回既有 booking state 或 duplicate rejection，不建立第二筆 active registration 或 ticket | Unique employee/event constraint evidence、UI duplicate handoff | AC-5 |
| P1-E2E-BKG-005 | employee | Full capacity 時仍超賣 | Given limited event 剩餘席次為 0 且 waitlist enabled | When eligible employee 報名 | Then 建立 waitlist registration，不核發 ticket，confirmed count 不增加 | Registration status、ticket absence、capacity count | AC-5, AC-6 |
| P1-E2E-BKG-006 | employee | 最後席次併發 booking 超賣 | Given 兩位 eligible employees 同時競爭最後 1 席 | When 兩筆 booking request 幾乎同時 commit | Then 只有一筆 registration 變 confirmed，另一筆依政策 waitlisted 或 rejected，PostgreSQL 仍是 final truth | DB integration 或 k6 release result、confirmed count、no oversell | AC-5, AC-14 |
| P1-E2E-BKG-007 | employee | Cross-city warning 變成隱性阻擋 | Given `employee-cross-city` 符合資格但城市不同於 event city | When 他查看 detail 並完成 booking | Then 顯示可讀的 cross-city warning，但 booking 允許通過，notification/audit context 不暴露不必要 PII | Event detail UI、booking confirmation、notification delivery | AC-4, AC-5, AC-10 |
| P1-E2E-BKG-008 | employee | 截止後仍可自行取消 | Given employee 在 registration close 後仍有 confirmed registration | When 他嘗試 self-cancellation | Then API 回 `409`，registration 與 ticket 維持 active，admin exception 才是唯一 audited path | Cancellation response、registration/ticket state、audit query | AC-5 |
| P1-E2E-BKG-009 | employee | No-show cooldown 沒有阻擋 limited booking | Given `employee-cooldown` 仍在 active limited-event cooldown 期間 | When 他報名 limited event | Then booking 以 cooldown reason 拒絕，且不建立 registration 或 ticket | Booking response、no registration/ticket rows | AC-5 |

## Unlimited and Family Count

| ID | Role | Risk | Given | When | Then | Evidence | AC |
|---|---|---|---|---|---|---|---|
| P1-E2E-FAM-001 | employee | Unlimited event 錯誤扣庫存或產生 companion ticket | Given unlimited event 接受 `family_count` | When eligible employee 以合法 family count 報名 | Then confirmed registration 保存 `family_count`，不扣 inventory，不建立 waitlist，也不核發可轉讓 companion ticket | Registration row、ticket count、capacity fields | AC-5, AC-8 |
| P1-E2E-FAM-002 | employee | Invalid family count 造成部分資料寫入 | Given unlimited event 有 family-count validation | When employee 提交 invalid count | Then booking 原子性拒絕，不建立 registration、ticket、notification 或成功 audit side effect | API response、DB row counts | AC-5 |
| P1-E2E-FAM-003 | employee | Limited event 意外建立 family tickets | Given limited event 每位 employee 最多一張 ticket | When employee 在 booking payload 帶 family members 或 companion count | Then request 依 API contract 被 validation reject 或忽略 unsupported fields，且不建立 companion ticket | API response、ticket count、audit if rejected | AC-5, AC-8 |

## Ticket Display and Ownership

| ID | Role | Risk | Given | When | Then | Evidence | AC |
|---|---|---|---|---|---|---|---|
| P1-E2E-TKT-001 | employee | Ticket handoff 開到錯誤票券 | Given employee 持有 active ticket | When 他從 booking success 或 ticket list 開啟 exact ticket detail URL | Then page 顯示該 ticket 的 event、holder context、non-transfer notice 與 QR image，但不把 raw signed token 當可見文字渲染 | Ticket detail UI、token redaction assertion | AC-8, AC-13 |
| P1-E2E-TKT-002 | employee | Missing 或 forbidden ticket fallback 到他人票券 | Given ticket detail URL 指向 missing 或 forbidden ticket | When page 載入 | Then 顯示可復原錯誤，不 fallback 顯示其他 ticket，也不渲染 QR | Ticket route UI、API response | AC-8, AC-13 |
| P1-E2E-TKT-003 | employee | Revoked 或 expired ticket 看起來仍可使用 | Given ticket 狀態為 revoked、expired、cancelled-equivalent 或其他 inactive state | When employee 開啟 ticket detail | Then UI 顯示 inactive state，且不呈現可用於 check-in 的 QR | Ticket state API、ticket detail UI | AC-8 |
| P1-E2E-TKT-004 | employee | Token forgery 取得票券資料 | Given ticket token 被竄改或與 employee 不匹配 | When token 用於 ticket lookup 或 check-in preparation | Then verification fails，且不暴露其他 employee 的 ticket payload | API response、signer/hash verification evidence | AC-8, AC-14 |

## Check-in and Offline Sync

| ID | Role | Risk | Given | When | Then | Evidence | AC |
|---|---|---|---|---|---|---|---|
| P1-E2E-CHK-001 | check-in staff | 有效 attendee 無法入場 | Given valid active ticket 尚未 redeemed | When check-in staff 線上掃描 signed token | Then exactly one successful check-in record 被建立，結果顯示 holder name、department、city、family count 供現場確認 | Live check-in route、check-in row、ticket state | AC-9 |
| P1-E2E-CHK-002 | check-in staff | Duplicate scan 被重複核銷 | Given ticket 已經 redeemed | When 同一 token 再次掃描 | Then response 是 duplicate 或 conflict，保留首次 redemption details，且不建立第二筆 accepted redemption | Live duplicate scan、DB unique constraint、audit/conflict row | AC-9 |
| P1-E2E-CHK-003 | check-in staff | Tampered token 被接受 | Given signed token 被竄改 | When check-in staff 提交 token | Then request 被拒絕，不建立 successful check-in record，invalid outcome 可稽核且不儲存 raw token | API response、check-in row absence、audit redaction | AC-8, AC-9, AC-12 |
| P1-E2E-CHK-004 | check-in staff | Holder mismatch 或 transfer attempt 不可見 | Given scanned ticket 不符合 expected holder context 或 transfer policy | When check-in staff 提交 holder/device context | Then check-in 被拒絕或標記，staff 看得到 mismatch reason，audit metadata 記錄 device 與 reason 但不含 full PII | Check-in response、audit metadata | AC-8, AC-9, AC-12 |
| P1-E2E-CHK-005 | check-in staff | Invalid ticket state 仍可入場 | Given ticket 是 revoked、expired 或 cancelled-equivalent | When token 被線上掃描 | Then redemption 被拒絕，ticket state 不變 | Check-in response、ticket state、audit row | AC-8, AC-9 |
| P1-E2E-CHK-006 | check-in staff | Offline package 取得過寬或過期權限 | Given check-in staff 下載 single-event offline package | When package 在 UI/API 被檢查 | Then package staff-bound、device-bound、time-bounded，且不授予 event governance 權限 | Offline package UI/API、forbidden admin route | AC-9, AC-13 |
| P1-E2E-CHK-007 | check-in staff | Offline sync 建立兩筆 accepted scans | Given offline scan 與已 online redeemed 或另一裝置 redeemed 的 ticket 衝突 | When offline sync 上傳 scan batch | Then first commit wins，conflicting scans 被標記 duplicate 或 conflict，並保存 conflict evidence 與 audit metadata | Offline sync response、conflict rows、audit query | AC-9 |
| P1-E2E-CHK-008 | check-in staff | API outage 造成本地假 committed state | Given check-in API 在 online redemption 或 offline sync 期間不可用 | When staff 提交 scan 或 sync batch | Then UI 顯示 controlled failure 或 pending sync state，系統不在 PostgreSQL 外自行發明 committed check-in truth | UI error state、readiness/k6 evidence、no accepted DB row | AC-9, AC-14 |

## Reports, Exports, Notifications, and Audit

| ID | Role | Risk | Given | When | Then | Evidence | AC |
|---|---|---|---|---|---|---|---|
| P1-E2E-RPT-001 | HR/system admin | HR 無法驗證參與狀況 | Given events 已有 confirmed、waitlisted、cancelled、no-show、ticket、family 與 check-in data | When HR 開啟 reports | Then 看得到 aggregate counts、family totals、city distribution、ticket counts、check-in counts 與 remaining capacity | Reports route、report API、DB aggregate comparison | AC-11 |
| P1-E2E-RPT-002 | HR/system admin | Export 洩漏 employee-level PII | Given HR 匯出 CSV report | When export ready | Then exported columns 限於 Phase 1 whitelist，且排除 employee rows、full names、email addresses、signed tokens、QR payloads、provider tokens、session data 與 raw audit metadata | Export artifact、object storage adapter、audit row | AC-11, AC-12 |
| P1-E2E-RPT-003 | employee | Unauthorized report access 洩漏資料 | Given employee 開啟 report/export/audit routes 或 APIs | When request 送出 | Then 回 `401` 或 `403`，不建立 export file，也不回傳 aggregate data | Forbidden route、API response、object storage absence | AC-11, AC-13 |
| P1-E2E-RPT-004 | activity admin or HR/system admin | Audit filters 只在 client 端過濾或排序不穩定 | Given audit logs 包含 event、eligibility、booking、cancellation、check-in、export、notification actions | When user 依 actor、role、action、entity、time range、limit 或 cursor 篩選 | Then filtering 在 server-side 執行，cursor ordering 穩定，metadata 已 redacted | Audit route、API response、pagination check | AC-12 |
| P1-E2E-RPT-005 | worker | Notification retry 重複投遞 | Given booking、cancellation、cross-city、ticket-state 或 report-export outbox events 存在 | When worker retry 或 replay delivery | Then delivery records idempotent，suppressed preferences 被遵守，dead-letter state 可見，raw PII 不寫入 logs | Worker tests、Mailhog delivery、delivery route、log scan | AC-10, AC-12 |
| P1-E2E-RPT-006 | worker | Backing-service outage 破壞 core booking truth | Given Mailhog、MinIO、Redis 或 worker processing 不可用 | When booking、check-in、report export 或 notification side effects 執行 | Then core booking/check-in state 不 fallback 到 local process memory，retryable side effects 會持久化或在 PostgreSQL 標記 failed | k6 smoke/release、worker state rows、readiness/log evidence | AC-10, AC-11, AC-14 |

## Authorization, Degraded States, and Layout

| ID | Role | Risk | Given | When | Then | Evidence | AC |
|---|---|---|---|---|---|---|---|
| P1-E2E-AUT-001 | unauthorized | Missing claims 被當成隱性授權 | Given provider claims missing、expired、tampered、incomplete 或 unmapped | When protected page 或 API 被 request | Then sensitive operations 回 `401` 或 `403`，任何 read-only degradation 都必須明確且已文件化 | Auth route/API response、forbidden UI | AC-13 |
| P1-E2E-AUT-002 | wrong role | Role navigation 暴露不可進入 workspace | Given role 只能進入一個 workspace | When shell render | Then inaccessible workspace switch options 與 protected links 不可見，direct deep links 仍顯示 forbidden state | Mocked E2E route coverage | AC-13 |
| P1-E2E-AUT-003 | all roles | Responsive layout 隱藏關鍵 action 或裁切文字 | Given employee、activity-admin、check-in、offline check-in、HR/reporting、audit、notifications、unauthorized routes | When 以 375px、768px、1024px、1440px render | Then 沒有 horizontal overflow、沒有 clipped visible button text、沒有 toolbar leakage，primary actions 仍可被找到 | `test:e2e` viewport matrix、overflow assertions | AC-13 |
| P1-E2E-AUT-004 | all roles | Error state 含糊且 retry 不安全 | Given loading、empty、validation error、forbidden、conflict 或 transient service error state 發生 | When UI render result | Then user 看得到穩定狀態，failed action 不被呈現為成功，retry 不會重複 committed records | UI state assertions、idempotency/backend tests | AC-5, AC-9, AC-13, AC-14 |

## Traceability Checklist

- 每個 testcase 至少對應一個 Phase 1 production AC 或 reviewer risk：
  eligibility recheck、oversell prevention、duplicate booking、idempotency、
  one-time check-in、report privacy、auditability、authorization、degraded
  backing services、responsive role coverage。
- 不能只用 happy path 當 production completion 證據。每個 critical
  correctness promise 在標記對應 AC complete 前，都必須有 negative 或
  failure scenario。
- Documentation-only coverage 不滿足 production completion。每個完成的 AC
  仍需要 implementation evidence、service 或 integration tests、user-facing
  行為的 Playwright coverage、適用的 k6 coverage、release evidence 與
  reviewer approval。
