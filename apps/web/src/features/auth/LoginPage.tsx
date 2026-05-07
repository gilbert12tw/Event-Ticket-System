import { localSSOPrincipals } from "@/app/routes";
import { roleLabel } from "@/lib/formatting";
import { Alert, StatusBadge } from "@/components/shared";
import { StatusPanel } from "@/components/layout";

export function LoginPage({
  health,
  ready,
  message,
  onLogin
}: {
  health: string;
  ready: string;
  message: string;
  onLogin: (principalID: string) => void;
}) {
  const groups = Array.from(new Set(localSSOPrincipals.map((principal) => principal.group)));
  return (
    <main className="login-shell">
      <section className="login-panel">
        <div className="login-copy">
          <div className="brand-block login-brand">
            <div className="brand-mark" aria-hidden="true">
              C
            </div>
            <div>
              <div className="brand-title">企業活動票務</div>
              <div className="brand-subtitle">Local SSO 模擬登入</div>
            </div>
          </div>
          <div>
            <div className="eyebrow">Phase 1 Access</div>
            <h1>選擇一個企業身分進入工作台</h1>
            <p>後端會簽發 HttpOnly SameSite session cookie；前端不再保存或傳送角色 header。</p>
          </div>
          <StatusPanel health={health} ready={ready} />
          {message && <Alert tone="warn">{message}</Alert>}
        </div>
        <div className="login-options" aria-label="Local SSO principals">
          {groups.map((group) => (
            <div className="principal-group" key={group}>
              <h2>{group}</h2>
              {localSSOPrincipals
                .filter((principal) => principal.group === group)
                .map((principal) => (
                  <button className="principal-card" type="button" key={principal.id} onClick={() => onLogin(principal.id)}>
                    <span>
                      <strong>{principal.label}</strong>
                      <small>{principal.id}</small>
                    </span>
                    <span>
                      <StatusBadge tone={principal.role === "employee" ? "info" : "neutral"}>{roleLabel(principal.role)}</StatusBadge>
                      <small>{principal.description}</small>
                    </span>
                  </button>
                ))}
            </div>
          ))}
        </div>
      </section>
    </main>
  );
}
