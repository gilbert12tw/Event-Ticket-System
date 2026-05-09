# Phase 1 NFR & Capacity

本文件補充 `docs/ARCHITECTURE.md` 與 `docs/specs/phase1-product-requirements.md` 的非功能與容量要求。
產品完成邊界以 `docs/specs/phase1-production-upper-bound.md` 為最後驗收依據。

## 1. 可用性與可維運

| ID | 要求 | 影響 |
| --- | --- | --- |
| NFR-HA-01 | 一般使用端 uptime 需維持良好可用；核心寫入/核銷流程優先穩定。 | 核心路徑不因次要服務失效而阻塞。 |
| NFR-HA-02 | 核心流程不落到單點：app / worker 可水平擴充。 | 容錯設計以 PostgreSQL + 唯一鍵限制為主。 |
| NFR-HA-03 | `Redis` 僅作快取/預留 gate，不得為最終交易真相來源。 | 交易結果以 DB 交易 + unique constraint + outbox 邏輯為準。 |
| NFR-HA-04 | 服務重啟可安全回收；進程以 env-config 為核心差異。 | 遵循 12-Factor：stateless process / port binding / env config。 |

## 2. 效能與一致性

| 操作 | P99 目標 |
| --- | --- |
| 活動列表 | < 200ms |
| 資格檢查 | < 300ms |
| 報名請求（交易段） | < 500ms |
| 線上核銷 | < 200ms |
| 票券產生 | < 2s（可非同步） |

- 報名與核銷必須透過 PostgreSQL 交易與約束保證一致性；快取只加速讀取，不可授權交易。
- booking/ticket/check-in/cancel 需可重試，返回同等結果（idempotency）。
- 票券一次核銷以 DB unique constraint 保底，衝突需保留首筆成功紀錄。

## 3. 安全與隱私

| ID | 要求 | 觀察 |
| --- | --- | --- |
| NFR-SEC-01 | 不保存密碼、不實作登入生命週期。 | 只驗證外部 token 與 claims。 |
| NFR-SEC-02 | API 與核銷輸入必需有 rate limit、驗證、錯誤邊界。 | 對重放、惡意輸入、暴力重試具防護。 |
| NFR-SEC-03 | PII 與 token 不可寫入明文 log。 | 日誌需 trace/actor/event/action/status 最小化最小資訊。 |
| NFR-SEC-04 | signed token 應綁定員工與票券，僅可在有效狀態與有效時間內使用。 | 防篡改 / 防截圖轉賣 / 防轉用。 |

## 4. 可觀測性

- 監控：RPS、錯誤率、lock wait、queue lag、剩餘名額、核銷成功率、衝突率、no-show 命中率。
- 日誌：JSON 結構化輸出，包含 `trace_id`、`action`、`status`，避免完整 PII。
- 追蹤：報名、抽籤、核銷、通知、離線同步需具 tracing 分區；失敗流程需可定位。

## 5. 可擴充性與容量

### 5.1 目標規模

| Phase | 員工規模 | 一般 DAU | 熱門活動同時段 |
| --- | ---: | ---: | ---: |
| Phase 1 | 10,000 | 500 / 日 | 2,000 人 |
| Phase 2 | 50,000 | 5,000 / 日 | 15,000 人 |
| Phase 3 | 90,000 | 18,000 / 日 | 54,000 人 |

### 5.2 峰值推估

| Phase | Raw App RPS | Booking TPS | Concurrent |
| --- | ---: | ---: | ---: |
| Phase 1 | 17.8 | 2.2 | 320 |
| Phase 2 | 133.3 | 16.7 | 2,400 |
| Phase 3 | 480 | 60 | 10,000 |

### 5.3 讀寫與瓶頸

- 小時維度流量普遍偏低，壓力集中於熱門活動開放前 12 分鐘。
- Peak writes 主要在 limited 活動，需交易保證；unlimited 可走無扣庫存流程。
- 報名前 12 分鐘壓力建議以：rate limit、idempotency、重試抑制、DB lock 行為監控、必要時預留快取作為保守保護。

## 6. 產品上限判斷

此文件只定義 NFR/容量門檻，未包含完整生產可交付項目。
跨 AZ、RTO/RPO、managed queue / service mesh / Kafka / multi-region，保留到後續 phase 或另行 spec。
對資安、可用性、效能與容量門檻的最終完成證明，請以 `docs/specs/phase1-production-upper-bound.md` 的驗收矩陣為準。
