import type { MockProfile } from "@/lib/api";
import { roleLabel } from "@/lib/formatting";
import { Alert, StatusBadge } from "@/components/shared";
import { StatusPanel } from "@/components/layout";

export function MockProfileSelector({
  health,
  ready,
  message,
  profiles,
  onSelect,
}: {
  health: string;
  ready: string;
  message: string;
  profiles: MockProfile[];
  onSelect: (profileID: string) => void;
}) {
  const groups = Array.from(
    new Set(profiles.map((profile) => profile.department || "Mock Profiles")),
  );
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
              <div className="brand-subtitle">Mock Provider Claims</div>
            </div>
          </div>
          <div>
            <div className="eyebrow">Metadata Profile</div>
            <h1>選擇一個模擬 provider profile</h1>
            <p>
              此入口只在 local、demo、test 可用；選擇後會由後端簽發 mock
              provider token，產品 shell 仍走 bearer claims。
            </p>
          </div>
          <StatusPanel health={health} ready={ready} />
          {message && <Alert tone="warn">{message}</Alert>}
        </div>
        <div className="login-options" aria-label="Mock provider profiles">
          {groups.map((group) => (
            <div className="principal-group" key={group}>
              <h2>{group}</h2>
              {profiles
                .filter(
                  (profile) =>
                    (profile.department || "Mock Profiles") === group,
                )
                .map((profile) => (
                  <button
                    className="principal-card"
                    type="button"
                    key={profile.profile_id}
                    onClick={() => onSelect(profile.profile_id)}
                  >
                    <span>
                      <strong>{profile.display_name}</strong>
                      <small>{profile.profile_id}</small>
                    </span>
                    <span>
                      <StatusBadge
                        tone={
                          profile.mapped_roles[0] === "employee"
                            ? "info"
                            : "neutral"
                        }
                      >
                        {roleLabel(profile.mapped_roles[0] || "employee")}
                      </StatusBadge>
                      <small>
                        {profile.job_title || profile.department} ·{" "}
                        {profile.site}
                      </small>
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
