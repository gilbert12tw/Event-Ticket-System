import type { MockProfile } from "@/lib/api";
import { roleLabel } from "@/lib/formatting";
import {
  Alert,
  DebugChromeGate,
  DebugToggle,
  StatusBadge,
} from "@/components/shared";
import { StatusPanel } from "@/components/layout";
import { Button } from "@/components/ui/button";
import { departmentLabel, siteLabel } from "@/lib/ui/options";
import { isDebugChromeAvailable } from "@/lib/ui/debug";

export function MockProfileSelector({
  debugChromeEnabled,
  health,
  ready,
  message,
  onToggleDebugChrome,
  profiles,
  onSelect,
}: {
  debugChromeEnabled: boolean;
  health: string;
  ready: string;
  message: string;
  onToggleDebugChrome: (enabled: boolean) => void;
  profiles: MockProfile[];
  onSelect: (profileID: string) => void;
}) {
  const groups = Array.from(
    new Set(profiles.map((profile) => profile.department || "本機身分")),
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
              <div className="brand-subtitle">本機身分入口</div>
            </div>
          </div>
          <div>
            <div className="eyebrow">本機身分</div>
            <h1>選擇一個本機身分</h1>
            <p>
              此入口只在本機環境可用；選擇後會由系統簽發臨時身分簽章，產品工作台仍使用身分宣告。
            </p>
          </div>
          {isDebugChromeAvailable() && (
            <DebugToggle
              enabled={debugChromeEnabled}
              onToggle={onToggleDebugChrome}
            />
          )}
          <DebugChromeGate enabled={debugChromeEnabled}>
            <StatusPanel health={health} ready={ready} />
          </DebugChromeGate>
          {message && <Alert tone="warn">{message}</Alert>}
        </div>
        <div className="login-options" aria-label="本機身分清單">
          {groups.map((group) => (
            <div className="principal-group" key={group}>
              <h2>{departmentLabel(group)}</h2>
              {profiles
                .filter(
                  (profile) => (profile.department || "本機身分") === group,
                )
                .map((profile) => (
                  <Button
                    className="principal-card"
                    type="button"
                    key={profile.profile_id}
                    onClick={() => onSelect(profile.profile_id)}
                    variant="ghost"
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
                        {profile.job_title ||
                          departmentLabel(profile.department)}{" "}
                        · {siteLabel(profile.site)}
                      </small>
                    </span>
                  </Button>
                ))}
            </div>
          ))}
        </div>
      </section>
    </main>
  );
}
