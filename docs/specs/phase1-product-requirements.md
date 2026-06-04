# Phase 1 Product Requirements

本文件為 Phase 1 產品需求來源之一，與以下文件協同使用：

- `docs/openapi.yaml`：API 契約（最終對外接口）。
- `docs/ARCHITECTURE.md`：模組邊界、部署與一致性原則。
- `docs/specs/phase1-production-upper-bound.md`：Phase 1 完成驗收標準（完成契約）。
- `docs/specs/phase1-nfr-and-capacity.md`：非功能需求與容量推估。

## 1. Phase 1 邊界

Phase 1 僅交付「可用、可驗證、可維護」的票務核心與現場驗票流程；不追求跨域高可用或多服務分拆。

- Phase 1 不做本地登入/登出、密碼、session 或 refresh token 生命週期管理；僅接受外部 provider token 並使用 claims 作為授權輸入。
- 活動海報上傳與瀏覽已支援；活動附件上傳與掃毒、Excel/PDF 匯出、ticket PDF、跨區 HA、RTO/RPO、跨區容災不列入產品交付範圍。
- `/api/v1/auth/login`、`/api/v1/auth/logout` 不是產品 OpenAPI 要求；local demo 改用 mock provider metadata profiles 產生 bearer token，僅限開發測試用途。
- HR sync 與營運設定在本階段維持為 same-codebase operator/admin command；產品 OpenAPI 僅暴露 eligibility impact review 查詢與 resolve flow，未新增 HR sync/settings API。

## 2. 使用者路徑

| Journey | 目標 | 核心需求 |
| --- | --- | --- |
| J1 活動建立與規則設定 | 福委建立活動並定義資格 | 狀態機、名額、地點、時間、票種、資格條件、公告範圍 |
| J2 員工報名與配票 | 員工查活動、確認資格、完成報名 | 即時資格驗證、剩餘名額/候補顯示、跨城市提示 |
| J3 電子票券與入場 | 員工顯示票券、現場驗票員核銷 | QR 簽章、不可轉讓、一次核銷 |
| J4 報表與稽核 | HR / 系統管理員查看 | 活動參與、到場、趨勢、audit log 可追溯 |

## 3. 功能需求

| ID | 需求 |
| --- | --- |
| FR-AUTH-01 | 系統只消費外部 provider / SSO claims，包含 `employee_id`、角色、部門、城市等必要欄位。 |
| FR-AUTH-02 | RBAC 依 claims 映射為 employee / activity admin / check-in staff / HR-admin。 |
| FR-AUTH-03 | 身份缺失或角色不符時，敏感操作必須拒絕並回傳可理解錯誤；可降級為只讀瀏覽。 |
| FR-EVENT-01 | 福委可建立與管理活動（草稿、上架、關閉、封存）、欄位含名稱、描述、時間、地點、城市、名額/名額類型、報名時段。 |
| FR-EVENT-02 | 活動列表與活動詳情需根據活動狀態與使用者資格顯示可報名/候補/不可報名。 |
| FR-EVENT-03 | 支援 limited / unlimited 兩種名額型態；limited 需容量，unlimited 不扣庫存。 |
| FR-QUAL-01 | 管理員可定義多維資格條件（部門、城市、職級、據點、年資、自訂標籤等）與 AND / OR / NOT 組合。 |
| FR-QUAL-02 | 每次報名前需即時驗證資格、冷卻狀態與活動條件，且最終授權需在下單時再次驗證。 |
| FR-QUAL-03 | 資格異動需留下 audit log；新異動可觸發受影響報名清單通知。 |
| FR-REG-01 | 支援先搶先得與抽籤配票；抽籤需可重現。 |
| FR-REG-02 | limited 報名需防超賣，且同員工同活動僅可成功拿到一張票；同時保留候補名單。 |
| FR-REG-03 | unlimited 報名可帶 family_count，不扣庫存且不產生可轉讓 companion ticket。 |
| FR-REG-04 | 員工可自行取消但僅限報名期間；逾期取消需經管理員敏感例外流程並記錄原因。 |
| FR-REG-05 | `cross-city` 僅屬警示，不得阻擋符合資格報名。 |
| FR-REG-06 | 員工多次申請需 idempotent；重複請求不會產生重複確定結果。 |
| FR-TKT-01 | 報名成功產生簽章 QR ticket；員工一次報名一筆有效票券。 |
| FR-TKT-02 | ticket 狀態需可見為：未使用、已核銷、已取消、已過期。 |
| FR-TKT-03 | 票券需可離線顯示，無網路時仍能出示 QR。 |
| FR-CHK-01 | 驗票員掃碼後可完成核銷，且同一票僅可成功一次。 |
| FR-CHK-02 | 重複掃描需回傳首筆核銷資訊並保留衝突記錄。 |
| FR-CHK-03 | 線上核銷需顯示持有者姓名、部門、城市、同行人數，供現場判斷轉讓風險。 |
| FR-NOTI-01 | 需至少支援 Email + 站內訊息；報名結果、抽籤結果、取消結果、跨城市提醒需可被通知。 |
| FR-REPORT-01 | HR 管理端可查看 dashboard、即時報表與城市分佈，並可輸出 CSV。 |
| FR-ADMIN-01 | 管理端可設定票券保留、抽籤排程、無到場冷卻與通知模板參數。 |
| FR-ADMIN-02 | HR / 管理員可查 audit log，並可篩選 actor、時間、操作類型。 |
| FR-ADMIN-03 | 敏感操作（事件變更、資格變更、撤銷票券、批次取消）必需留存 immutable audit。 |

## 4. 驗證前置條件（最小）

- 所有狀態變更以 PostgreSQL 為最終授權與一致性來源（交易 + 約束）。
- booking、取消、核銷與通知投遞必須具備 idempotency / 去重機制。
- 核心流程不依賴快取結果授權，必須在交易內重查事件狀態與資格。
- API 回傳使用 `docs/openapi.yaml` 定義的 envelope 與錯誤格式。
