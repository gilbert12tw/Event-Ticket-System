# 🏛️ Architecture Guide — Corporate Event Ticketing System

> 本文件為 CETS 系統的整體架構指南，涵蓋設計原則、技術選型、模組劃分、資料模型、部署策略、可觀測性、與安全性。

---

## 目錄
1. [設計原則](#1-設計原則)
2. [架構總覽](#2-架構總覽)
3. [技術選型](#3-技術選型)
4. [Monorepo 與模組劃分](#4-monorepo-與模組劃分)
5. [微服務拆分與職責](#5-微服務拆分與職責)
6. [資料模型與一致性策略](#6-資料模型與一致性策略)
7. [Hot Event 訂票核心設計](#7-hot-event-訂票核心設計)
8. [事件驅動 (Event-Driven) 設計](#8-事件驅動-event-driven-設計)
9. [雲端基礎建設與部署](#9-雲端基礎建設與部署)
10. [CI/CD 流水線](#10-cicd-流水線)
11. [可觀測性 (Observability)](#11-可觀測性-observability)
12. [安全性 (Security)](#12-安全性-security)
13. [可靠性與災難復原 (Resilience & DR)](#13-可靠性與災難復原-resilience--dr)
14. [全球化部署 (Multi-Region)](#14-全球化部署-multi-region)
15. [敏捷開發流程](#15-敏捷開發流程)
16. [架構決策紀錄 (ADR)](#16-架構決策紀錄-adr)

---

## 1. 設計原則

| 原則 | 說明 |
|------|------|
| **Cloud Native First** | 所有服務皆為容器化、無狀態、12-factor app；以 K8s 為運算平台 |
| **API-First** | OpenAPI / gRPC schema 先行，前後端可平行開發 |
| **Microservices** | 依領域 (DDD) 拆分，每個服務擁有獨立資料庫 (Database-per-service) |
| **Event-Driven** | 跨服務通訊以事件為主，降低耦合 |
| **Fail Fast, Recover Faster** | 透過熔斷、超時、重試、idempotency 確保彈性 |
| **Everything as Code** | 基礎建設、配置、流水線、文件全部納入版本控制 |
| **Shift-Left Quality** | Lint、型別、測試、Security Scan 全在 PR 階段攔截 |
| **GitOps** | 部署狀態以 Git 為唯一真實來源 (Single Source of Truth) |
| **Observable by Default** | 所有服務內建 metrics、logs、traces |

> 12-Factor 實作規則、程式碼範例與合規檢查清單詳見 **[AGENTS.md](../AGENTS.md)**。

---

## 2. 架構總覽

### 2.1 高階架構 (C4 - Context & Container)

```
                          ┌─────────────────────────────┐
                          │   外部認證系統 (SSO/OIDC)    │
                          └──────────────┬──────────────┘
                                         │ JWT
              ┌────────────┬─────────────┼─────────────┬────────────┐
              │            │             │             │            │
         ┌────▼────┐  ┌────▼────┐   ┌───▼─────┐   ┌───▼────┐   ┌──▼──────┐
         │ Employee │  │  Admin  │   │   HR    │   │  現場   │   │ Mobile  │
         │  Web    │  │  Portal │   │Dashboard│   │ 核銷裝置│   │  PWA    │
         └────┬────┘  └────┬────┘   └────┬────┘   └────┬───┘   └──┬──────┘
              └────────────┴──────┬──────┴─────────────┴──────────┘
                                  │ HTTPS
                          ┌───────▼────────┐
                          │   CDN / WAF    │
                          └───────┬────────┘
                                  │
                          ┌───────▼────────────────────┐
                          │ API Gateway (Kong/Nginx)   │
                          │  - JWT 驗證 / 角色路由      │
                          │  - Rate Limit / Circuit     │
                          └─┬─────┬─────┬─────┬─────┬──┘
                            │     │     │     │     │
                            │     │     │     │     │
                       ┌────▼─┐ ┌─▼──┐ ┌▼───┐ ┌▼──┐ ┌▼──────┐
                       │Event │ │Book│ │Ticket│ │Not│ │Analyt│
                       │ Svc  │ │Svc │ │ Svc  │ │if │ │ ics   │
                       └──┬───┘ └─┬──┘ └──┬──┘ └─┬─┘ └──┬───┘
                          │       │       │      │      │
                          ▼       ▼       ▼      ▼      ▼
                       ┌────────────────────────────────────┐
                       │    Kafka / RabbitMQ (Event Bus)    │
                       └────────────────────────────────────┘
                                          │
                  ┌──────────────┬────────┴────────┬─────────────┐
              ┌───▼────┐    ┌────▼─────┐     ┌────▼────┐   ┌────▼────┐
              │Postgres│    │  Redis   │     │   S3    │   │ ELK /   │
              │  (RDS) │    │(Cache+鎖)│     │ Object  │   │ Loki    │
              └────────┘    └──────────┘     └─────────┘   └─────────┘
```

### 2.2 部署視圖

```
┌──────────────────────────────────────────────────────────────┐
│                    Kubernetes Cluster (EKS)                   │
│                                                               │
│  ┌─────────────────┐  ┌─────────────────┐  ┌──────────────┐ │
│  │ Ingress Tier     │  │ Service Tier    │  │ Data Tier    │ │
│  │ (Nginx/Kong)     │→ │ (microservices) │→ │ (StatefulSet)│ │
│  │ HPA: 2-10 pods   │  │ HPA: 3-30 pods  │  │ + PVC        │ │
│  └─────────────────┘  └─────────────────┘  └──────────────┘ │
│                                                               │
│  ┌─────────────────────────────────────────────────────────┐ │
│  │  Service Mesh (Istio - optional)                        │ │
│  │  - mTLS / Traffic Splitting / Canary                    │ │
│  └─────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────┘
```

---

## 3. 技術選型

### 3.1 前端
| 類別 | 選擇 | 理由 |
|------|------|------|
| Framework | **Next.js 14 (App Router)** | SSR / SSG / RSC、SEO 友善、Vercel 部署簡易 |
| UI 函式庫 | **shadcn/ui + Radix + TailwindCSS** | 可客製、可訪問性 (a11y) 佳 |
| 狀態管理 | **Zustand + TanStack Query** | 輕量、Server state 與 Client state 分離 |
| 表單 | **React Hook Form + Zod** | 型別安全、效能佳 |
| i18n | **next-intl** | 為全球化做準備 |
| Build | **Turbopack** | 高速建置 |

### 3.2 後端
| 類別 | 選擇 | 理由 |
|------|------|------|
| 主要語言 | **TypeScript (NestJS)** | 與前端共享型別、生態完整 |
| 高併發服務 | **Go (Gin/Fiber)** | Booking Service 需高吞吐 |
| API 通訊 | **gRPC (內部) + REST (外部)** | 型別安全、效能 |
| ORM | **Prisma (TS) / GORM (Go)** | 類型安全 migration |
| Schema | **Protobuf + OpenAPI 3.1** | 契約先行 |

### 3.3 資料層
| 類別 | 選擇 | 理由 |
|------|------|------|
| 主資料庫 | **PostgreSQL 16** | ACID、JSONB、Row Locking 強 |
| 快取 / 鎖 | **Redis 7** | 分散式鎖 (Redlock)、快取 |
| 訊息佇列 | **Kafka** | 高吞吐、訊息保留、事件溯源 |
| Object Storage | **MinIO** (本地) / **S3** (生產) | 票券 PDF、活動圖片 |
| Search (選用) | **OpenSearch** | 活動搜尋 |

### 3.4 基礎建設
| 類別 | 選擇 |
|------|------|
| 本地開發 | **Docker Compose** (Postgres / Redis / Kafka / MinIO) |
| 雲端 (生產) | **AWS** (EKS / RDS / ElastiCache / MSK / S3 / CloudFront) |
| IaC | **Terraform + Terragrunt** |
| K8s 部署 | **Helm + Kustomize** |
| GitOps | **ArgoCD** |
| Container Registry | **GHCR** (CI / 開發) / **ECR** (生產) |
| Secrets | **`.env` 檔案** (本地) / **AWS Secrets Manager** (生產) |

### 3.5 可觀測性
| 類別 | 選擇 |
|------|------|
| Metrics | **Prometheus + Grafana** |
| Logs | **Loki (or ELK)** |
| Traces | **OpenTelemetry + Tempo / Jaeger** |
| APM | **Grafana OSS** |
| Alert | **Alertmanager** |

### 3.6 開發工具
| 類別 | 選擇 |
|------|------|
| Monorepo | **Turborepo + pnpm workspaces** |
| Linter | **ESLint + Prettier + Biome** |
| Test | **Vitest + Playwright + k6 + Testcontainers** |
| Commit Lint | **commitlint + husky** |
| Docs | **MkDocs / Docusaurus (optional)** |

---

## 4. Monorepo 與模組劃分

### 4.1 為何選擇 Monorepo
- 共享型別 (Protobuf / TS types) 避免漂移
- 原子化跨服務變更 (例: API contract 同時更新前後端)
- 統一的 lint / test / CI 配置
- 一鍵跑全套整合測試

### 4.2 工具鏈
- **pnpm workspaces** — 套件管理、節省磁碟
- **Turborepo** — 增量建置、平行執行、遠端快取
- **Changesets** — 版本管理與 changelog

### 4.3 目錄職責

| 路徑 | 職責 |
|------|------|
| `apps/` | 前端 / End-user 應用 |
| `services/` | 後端微服務 (各自可獨立部署) |
| `packages/` | 跨應用共用程式碼 (型別、UI、設定) |
| `infra/` | IaC (Terraform / Helm / K8s) |
| `docs/` | 架構文件、ADR、API 規格 |
| `tools/` | 開發者腳本、CLI |

---

## 5. 微服務拆分與職責

採用 **領域驅動設計 (DDD)** 拆分服務，每個服務擁有獨立資料庫。

### 5.1 服務矩陣

| 服務 | 領域 | 資料 | 對外 API | 訂閱事件 | 發布事件 |
|------|------|------|----------|----------|----------|
| **api-gateway** | BFF / 路由 | — | REST | — | — |
| **event-service** | 活動管理 | events, rules | gRPC, REST | — | `EventCreated`, `EventUpdated` |
| **booking-service** | 訂票核心 | bookings, locks | gRPC, REST | `EventCreated` | `BookingRequested`, `BookingConfirmed` |
| **ticket-service** | 票券核銷 | tickets, validations | gRPC, REST | `BookingConfirmed` | `TicketIssued`, `TicketValidated` |
| **notification-service** | 通知 | (無持久化) | — | `BookingConfirmed`, `TicketIssued` | — |
| **analytics-service** | 報表 (CQRS read model) | analytics_db | REST | All domain events | — |

### 5.2 服務邊界規則
- ❌ 服務間 **不可** 直接存取對方資料庫
- ✅ 同步呼叫用 **gRPC** (內部) 或 **REST** (外部)
- ✅ 跨服務狀態變更走 **事件總線** (Kafka)
- ✅ 每個服務自帶 `health`, `ready`, `metrics` endpoint

---

## 6. 資料模型與一致性策略

### 6.1 核心實體 (簡化 ERD)

```
┌─────────────┐       ┌─────────────┐       ┌─────────────┐
│   Event     │ 1───* │ EventRule   │       │   Booking   │
│─────────────│       │─────────────│       │─────────────│
│ id (UUID)   │       │ id          │       │ id          │
│ title       │       │ event_id    │       │ event_id    │
│ description │       │ type        │       │ employee_id │
│ start_at    │       │ value       │       │ status      │
│ end_at      │       │ (region/    │       │ (PENDING/   │
│ capacity    │       │  quota/...)│       │  APPROVED/  │
│ status      │       └─────────────┘       │  REJECTED)  │
│ created_by  │                              │ created_at  │
└──────┬──────┘                              └──────┬──────┘
       │                                            │
       │ 1                                        1 │
       │                                            │
       └────────────* ┌─────────────┐ *─────────────┘
                      │   Ticket    │
                      │─────────────│
                      │ id          │
                      │ booking_id  │
                      │ qr_code     │
                      │ issued_at   │
                      │ validated_at│
                      │ status      │
                      └─────────────┘
```

### 6.2 一致性策略

| 場景 | 策略 |
|------|------|
| 單一服務內 | **強一致 (ACID)** — Postgres transaction |
| 跨服務 | **最終一致 (Eventual Consistency)** — 透過 Outbox Pattern + Kafka |
| 防止超賣 | **悲觀鎖 (`SELECT FOR UPDATE`) + Redis 分散式鎖雙保險** |
| 重複請求 | **Idempotency Key** (Header + Redis 24h TTL) |
| Saga | **Choreography-based** (booking → ticket → notification) |

### 6.3 Outbox Pattern

```
┌─ Tx Begin ─────────────────────────┐
│  1. UPDATE booking SET status=...   │
│  2. INSERT INTO outbox (event)      │
└─ Tx Commit ────────────────────────┘
                │
                ▼
        Outbox Relay (CDC / Debezium)
                │
                ▼
            Kafka Topic
```
保證資料庫變更與事件發布的原子性。

---

## 7. Hot Event 訂票核心設計

> 對應 Advanced Requirement 之效能、可靠性、正確性挑戰。

### 7.1 問題場景
熱門活動開放搶票時，可能發生：
- **超賣** — 多執行緒同時扣減庫存
- **慢查詢** — 大量併發壓垮資料庫
- **不公平** — 重試風暴導致先到者反而失敗

### 7.2 解決方案：分層庫存控制

```
[使用者] ──► [Rate Limiter (per user)] ──► [Token Bucket]
                                              │
                                              ▼
                              [Redis Pre-check 庫存]
                              (DECR if > 0, atomic Lua script)
                                              │
                                       ✓     OK    ✗ Fail-fast
                                              ▼
                              [Kafka: BookingRequested]
                                              │
                                              ▼
                              [Booking Worker (consumer)]
                                              │
                                              ▼
                              ┌───────────────────────────┐
                              │ Postgres Tx               │
                              │  SELECT ... FOR UPDATE    │
                              │  INSERT booking           │
                              │  UPDATE event capacity    │
                              │  INSERT outbox            │
                              └───────────────────────────┘
                                              │
                                              ▼
                              [Kafka: BookingConfirmed]
```

### 7.3 關鍵技術
- **Redis Atomic Lua** — 庫存預扣，毫秒級回應
- **訊息佇列削峰** — Kafka 緩衝瞬間流量
- **分散式鎖 (Redlock)** — 防止 race condition
- **DB 樂觀鎖 + version 欄位** — 最終一致性檢查
- **冪等性 (Idempotency)** — `client_request_id` 唯一索引
- **Circuit Breaker** — 上游故障時快速失敗

### 7.4 效能目標
- 單一熱門活動：**5,000 RPS、p99 < 500ms**
- 超賣發生率：**0%** (透過 DB constraint 兜底)

---

## 8. 事件驅動 (Event-Driven) 設計

### 8.1 事件目錄

```yaml
domain.event.v1:
  - EventCreated
  - EventUpdated
  - EventCancelled
  - BookingRequested
  - BookingConfirmed
  - BookingRejected
  - TicketIssued
  - TicketValidated
  - TicketExpired
```

### 8.2 主題設計
- **Topic 命名**：`<domain>.<entity>.<event>` (e.g. `booking.booking.confirmed`)
- **Partitioning**：以 `event_id` 為 key 確保同一活動事件順序
- **Schema Registry**：Avro / Protobuf，向前/向後相容
- **DLQ (Dead Letter Queue)**：處理失敗訊息隔離

### 8.3 訂票流程 Sequence

```
員工         API GW       Booking      Redis      Kafka       Ticket      Notif
 │             │             │           │          │           │           │
 │  POST /book │             │           │          │           │           │
 ├────────────►│             │           │          │           │           │
 │             ├────────────►│           │          │           │           │
 │             │             ├──pre-chk──►          │           │           │
 │             │             │◄──ok──────│          │           │           │
 │             │             ├──publish─────────────►           │           │
 │             │             │           │          │           │           │
 │             │◄── 202 ─────│           │          │           │           │
 │◄─ accepted ─│             │           │          │           │           │
 │             │             │           │          ├──consume─►│           │
 │             │             │           │          │           ├─issue tkt │
 │             │             │           │          │◄─publish──│           │
 │             │             │           │          │           │           │
 │             │             │           │          ├──consume──────────────►
 │             │             │           │          │           │           │
 │◄────────────  Email / IM 通知 ────────────────────────────────────────────│
```

---

## 9. 雲端基礎建設與部署

> **本章節描述生產環境架構。** 本地開發請使用 `infra/docker/docker-compose.dev.yml` 啟動所有依賴服務，無需任何雲端資源。

### 9.1 雲資源 (Terraform 管理)

```
┌────────────────────────────────────────────────┐
│                  AWS Account                    │
│                                                 │
│  ┌─ VPC (10.0.0.0/16) ────────────────────┐   │
│  │   ┌─ Public Subnet (3 AZ) ─┐            │   │
│  │   │  ALB / NAT             │            │   │
│  │   └────────────────────────┘            │   │
│  │   ┌─ Private Subnet (3 AZ) ┐            │   │
│  │   │  EKS Nodes / Lambda    │            │   │
│  │   └────────────────────────┘            │   │
│  │   ┌─ Data Subnet (3 AZ) ───┐            │   │
│  │   │  RDS / ElastiCache /MSK│            │   │
│  │   └────────────────────────┘            │   │
│  └──────────────────────────────────────────┘   │
│                                                  │
│  CloudFront ─► S3 (static) / ALB (api)          │
│  Route53 ─► DNS                                 │
│  WAF ─► Shield (DDoS)                           │
└─────────────────────────────────────────────────┘
```

### 9.2 K8s 工作負載
- **Namespace 隔離**：`dev`, `staging`, `prod`
- **HPA (Horizontal Pod Autoscaler)**：依 CPU + 自訂指標 (RPS) 擴縮
- **PDB (Pod Disruption Budget)**：保證最低可用副本數
- **NetworkPolicy**：服務間最小權限通訊
- **Resource Limits**：所有 pod 必須宣告 requests/limits

### 9.3 環境策略

| 環境 | 用途 | 部署方式 | 資料 |
|------|------|----------|------|
| **dev** | 開發測試 | 自動 (push to feature) | Mock |
| **staging** | 預生產 | 自動 (merge to main) | 匿名化 prod 資料 |
| **prod** | 正式 | 手動 approval | 真實 |

---

## 10. CI/CD 流水線

### 10.1 CI (PR Pipeline)

```yaml
on: pull_request
jobs:
  - lint            # ESLint + Prettier
  - typecheck       # tsc --noEmit
  - unit-test       # Vitest (with coverage gate ≥ 80%)
  - integration     # Testcontainers (real Postgres/Redis)
  - sast            # Semgrep / CodeQL
  - sca             # Snyk (dependency)
  - secret-scan     # Gitleaks
  - build-image     # Docker build + Trivy scan
  - contract-test   # Pact verification
```

> **PR Gate**：所有 job 通過 + 1 reviewer approval + commit 簽署 (DCO/GPG)

### 10.2 CD (Merge Pipeline — GitOps)

```
main branch ─┐
             ├─► Build & tag image (semver)
             ├─► Push to GHCR
             ├─► Update Helm values in `infra/` repo
             ├─► ArgoCD detects diff ─► sync to cluster
             └─► Smoke test (synthetic monitoring)
                       │
                       ▼
             [Staging E2E] ─► [Manual Approval] ─► [Prod Canary 10% → 50% → 100%]
```

### 10.3 部署策略
- **Canary Release** — Argo Rollouts，依 metrics 自動推進
- **Feature Flags** — Unleash (OSS)，與部署解耦
- **Blue-Green** — 重大變更使用
- **回滾** — `helm rollback` 或 ArgoCD UI 一鍵

---

## 11. 可觀測性 (Observability)

### 11.1 三大支柱

| 支柱 | 工具 | 範例 |
|------|------|------|
| **Metrics** | Prometheus | RPS, p50/p95/p99 延遲, 錯誤率, 庫存量 |
| **Logs** | Loki | 結構化 JSON, trace_id 關聯 |
| **Traces** | Tempo + OpenTelemetry | 跨服務呼叫鏈、慢查詢 |

### 11.2 RED Method (服務健康)
- **R**ate — 每秒請求數
- **E**rror — 錯誤率
- **D**uration — 延遲分佈

### 11.3 USE Method (資源)
- **U**tilization, **S**aturation, **E**rrors — CPU / Memory / Disk / Network

### 11.4 SLO 與告警

| 服務 | SLI | SLO | 錯誤預算 |
|------|-----|-----|---------|
| api-gateway | 成功率 | 99.9% | 0.1% / 30d |
| booking-service | p95 延遲 | < 300ms | — |
| ticket-service | 核銷成功率 | 99.95% | 0.05% / 30d |

### 11.5 Dashboards
- **Service Dashboard** — 每服務一個 (RED + 業務指標)
- **Business Dashboard** — 訂票數、轉換率、活動熱度
- **Infra Dashboard** — K8s 節點、Pod 狀態、資料庫連線

---

## 12. 安全性 (Security)

### 12.1 縱深防禦

| 層 | 控制 |
|---|------|
| **Edge** | CloudFront + WAF + Shield (DDoS) |
| **Network** | VPC、Private Subnet、Security Group、NetworkPolicy |
| **Identity** | OIDC SSO、JWT (短效 + refresh)、MFA for admin |
| **Authz** | RBAC (Employee / Admin / HR)、ABAC (region-based 規則) |
| **Data** | At-rest encryption (KMS)、In-transit (TLS 1.3) |
| **Secret** | AWS Secrets Manager + External Secrets Operator，**禁止 hardcode** |
| **Code** | SAST (Semgrep)、DAST (ZAP)、SCA (Snyk)、Container scan (Trivy) |
| **Runtime** | Falco / GuardDuty (異常行為偵測) |
| **Audit** | 所有寫操作記錄 audit log，保留 1 年 |

### 12.2 OWASP Top 10 對應
- ✅ Broken Access Control — 中央 Policy Decision Point
- ✅ Cryptographic Failures — TLS、KMS
- ✅ Injection — Prepared Statements、輸入驗證 (Zod)
- ✅ Insecure Design — Threat Modeling 每個 Cycle
- ✅ Security Misconfig — IaC scan (Checkov)、CIS Benchmark
- ✅ Vulnerable Components — Renovate / Dependabot 自動 PR
- ✅ Auth Failures — 短效 token、refresh rotation
- ✅ Software & Data Integrity — Image signing (Cosign)、SBOM
- ✅ Logging & Monitoring Failures — 中央化 logging + alerting
- ✅ SSRF — 出站防火牆 + 白名單

### 12.3 隱私
- **PII 資料**：員工編號、Email 加密存放
- **GDPR-ready**：資料刪除 API、保留期限
- **Audit Trail**：所有 PII 存取記錄

---

## 13. 可靠性與災難復原 (Resilience & DR)

### 13.1 彈性模式
- **Circuit Breaker** (Resilience4j / Polly)
- **Retry with Exponential Backoff**
- **Timeout** (每個外部呼叫必設)
- **Bulkhead** (資源隔離)
- **Graceful Shutdown** (`preStop` hook)

### 13.2 備援
- **DB**：RDS Multi-AZ + 自動快照 (PITR 7 天)
- **Cache**：Redis Cluster + Replica
- **MQ**：Kafka 3-broker, RF=3
- **App**：每個服務最少 3 副本 (跨 AZ)

### 13.3 DR 演練
- **RTO**：1 小時 (跨 region failover)
- **RPO**：5 分鐘 (logical replication)
- **Game Day**：每季一次 chaos engineering (Litmus / Chaos Mesh)

---

## 14. 全球化部署 (Multi-Region)

> 對應評分項：「如何做到全球化部署？」

### 14.1 部署模型：Active-Active

```
        ┌──────────────── Route53 (Geo Routing) ────────────────┐
        │                                                         │
   ┌────▼─────┐                                            ┌─────▼────┐
   │  US-East │                                            │  AP-NE-1 │
   │  Cluster │  ◄────── Cross-Region Replication ──────► │  Cluster │
   └──────────┘                                            └──────────┘
        │                                                         │
   ┌────▼─────┐                                            ┌─────▼────┐
   │ Postgres │  ──── Logical Replication (Debezium) ───►│ Postgres │
   └──────────┘                                            └──────────┘
```

### 14.2 策略
- **CDN Edge Caching** — CloudFront 全球節點
- **Geo-DNS** — 員工就近連線
- **資料分區 (Sharding by Region)** — 大型部署
- **冪等寫入 + CRDTs** — 跨 region 衝突解決
- **時區處理** — 一律存 UTC，前端轉換

---

## 15. 敏捷開發流程

### 15.1 Linear 工作流程

使用 **Linear** 作為唯一工作追蹤工具，Cycle 長度 2 週。

| 概念 | Linear 對應 |
|------|-------------|
| 里程碑 | Project / Milestone |
| 迭代 | Cycle (2 週) |
| 功能需求 | Issue |
| 子任務 | Sub-issue |
| Issue ID | `CETS-NNN`（自動生成） |

**Issue 狀態流**：Backlog → Todo → In Progress → In Review → Done

### 15.2 工作管理
- **Tool**：Linear
- **層次**：Project → Cycle → Issue → Sub-issue
- **DoR (Definition of Ready)**：AC 清楚、設計完成、相依釐清
- **DoD (Definition of Done)**：通過測試、Code Review、文件更新、部署到 staging

### 15.3 Spec 與驗收標準

Spec Template、Given/When/Then 格式、Edge Case 表、NFR 表、API Contract 格式，以及 12-Factor 合規檢查清單，統一定義於 **[AGENTS.md](../AGENTS.md)**。

### 15.4 分支策略：**Trunk-Based Development**
- `main` 永遠可部署
- 短期 feature branch (< 2 天)，命名格式：`feature/cets-NNN-short-description`
- Feature Flag 控制未完成功能
- **不使用 GitFlow** (對 CI/CD 不友善)

### 15.5 程式碼品質
- **PR < 400 行** (易於審查)
- **PR 標題帶 Linear Issue ID**：`feat(booking): add lock [CETS-42]`
- **Code Review SLA**：4 小時內首次回應
- **Pair / Mob Programming** — 複雜功能採用
- **重構**：每 Cycle 預留 20% 容量

---

## 16. 架構決策紀錄 (ADR)

所有重大決策以 ADR 形式存放在 `docs/adr/`。

### 模板

```markdown
# ADR-NNNN: <決策標題>

- **狀態**：Proposed / Accepted / Deprecated / Superseded
- **日期**：YYYY-MM-DD
- **決策者**：<人員>

## Context
<為何需要做這個決定？>

## Decision
<決定了什麼？>

## Consequences
<好處 / 壞處 / Trade-offs>

## Alternatives Considered
<其他方案與否決理由>
```

### 已建立 ADR (範例)
- `ADR-0001` — 採用 Monorepo (Turborepo)
- `ADR-0002` — 主資料庫使用 PostgreSQL
- `ADR-0003` — Booking Service 使用 Go 而非 Node.js
- `ADR-0004` — 使用 Kafka 作為事件總線
- `ADR-0005` — 採用 ArgoCD 進行 GitOps 部署

---

## 附錄 A — 評分標準對應

| 評分項 (%) | 對應章節 |
|-----------|---------|
| 30% 需求轉換與實作 | §5 服務拆分、§15.3 User Story |
| 10% 程式碼品質 | §10.1 PR Gate、§12 安全性、§15.5 |
| 25% 架構與可擴展性 | §2 架構總覽、§7 Hot Event、§9 K8s、§14 Multi-Region |

| 25% 系統測試與驗證 | §10.1 Test Pyramid、§13.3 Chaos、附錄 B |
| 10% 運維與可靠性 | §11 Observability、§13 Resilience |

## 附錄 B — 測試金字塔

```
                  ┌─────────┐
                  │   E2E   │  ← 少量、慢、貴 (Playwright)
                  └─────────┘
                ┌─────────────┐
                │ Integration │  ← Testcontainers
                └─────────────┘
            ┌───────────────────┐
            │      Unit         │  ← 大量、快、便宜 (Vitest)
            └───────────────────┘
        ┌───────────────────────────┐
        │ Static Analysis (lint/tsc)│
        └───────────────────────────┘
```

額外:
- **Contract Test** (Pact) — 服務間契約
- **Load Test** (k6) — 訂票尖峰
- **Chaos Test** (Chaos Mesh) — 故障注入
- **Security Test** (ZAP) — 滲透
