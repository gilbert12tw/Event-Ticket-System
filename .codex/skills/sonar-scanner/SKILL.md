---
name: sonar-scanner
description: Use when Codex needs to diagnose, install guidance, bootstrap, or run local SonarQube Server and SonarScanner CLI workflows in this project, especially when sonar-scanner, SONAR_HOST_URL, SONAR_TOKEN, Docker, Homebrew, or Sonar quality-gate evidence is missing. Prefer this skill for SonarQube/SonarScanner setup, local scan readiness, scanner CLI troubleshooting, and Event-Ticket-System Sonar commands.
---

# Sonar Scanner

Use this skill to get the local Sonar path ready without leaking tokens or changing the machine unexpectedly.

## Workflow

1. Inspect the repo first.
   - Read `sonar-project.properties`, `package.json`, and any Sonar wrapper scripts before suggesting commands.
   - In this repo, prefer `pnpm sonar:scan` for normal local scans.
   - For Phase 3 evidence, prefer `scripts/compose/phase3-sonar-result.sh`; it writes the result report consumed by `scripts/compose/phase3-quality.sh`.

2. Run the read-only diagnostic helper when filesystem access is available:

   ```bash
   bash .codex/skills/sonar-scanner/scripts/diagnose_sonar.sh
   ```

   Use its output to decide which setup step is missing. The helper must not install tools, start containers, or print `SONAR_TOKEN`.

3. Verify required scan inputs.
   - `sonar-project.properties` must exist, or scanner properties must be passed with `sonar-scanner -D...`.
   - `SONAR_HOST_URL` must point to a reachable SonarQube Server or SonarQube Cloud endpoint.
   - `SONAR_TOKEN` must be set and non-empty; never echo, log, commit, or paste the token.
   - Coverage should be generated before scanning. In this repo, `pnpm sonar:scan` runs `pnpm test:coverage && sonar-scanner`.

4. Fix the missing piece with the least invasive option.
   - If `sonar-scanner` exists, use the repo scan command.
   - If `sonar-scanner` is missing on macOS and Homebrew is available, suggest `brew install sonar-scanner`.
   - If no local SonarQube Server is available and Docker is available, suggest a local evaluation server:

     ```bash
     docker run -d --name sonarqube -e SONAR_ES_BOOTSTRAP_CHECKS_DISABLE=true -p 9000:9000 sonarqube:latest
     ```

   - For repeated local server use, prefer persistent Docker volumes for `/opt/sonarqube/data`, `/opt/sonarqube/extensions`, and `/opt/sonarqube/logs`.
   - If using the Docker scanner image instead of installing the CLI, account for host networking. On macOS, a scanner container usually reaches a host SonarQube server through `http://host.docker.internal:9000`, not `http://localhost:9000`.

5. Request approval before side effects when the active environment requires it.
   - Installing packages, pulling Docker images, starting containers, or changing shell profile files are side effects.
   - Do not hide installation or Docker startup inside scripts.

## Token And Secret Rules

- Treat `SONAR_TOKEN` as a secret even when it is short-lived.
- Report only whether the token is present, never its value or length.
- Do not write tokens into `sonar-project.properties`, `.env`, command history examples, or result reports.
- Prefer environment variables over inline command-line token arguments.

## Expected Commands In This Repo

Use these only after prerequisites are present:

```bash
pnpm sonar:scan
```

```bash
scripts/compose/phase3-sonar-result.sh
```

For the Phase 3 wrapper, also confirm `curl`, `jq`, `pnpm`, `sonar-scanner`, `SONAR_HOST_URL`, and `SONAR_TOKEN` are available.

## Documentation Freshness

When changing this skill or answering version-sensitive Sonar setup questions, fetch current SonarSource docs with Context7 before giving final commands. SonarScanner CLI and SonarQube Server installation details can change.
