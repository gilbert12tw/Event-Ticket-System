---
name: event-ticketing-requirements-reviewer
description: Review Corporate Event Ticketing System requirements, architecture drafts, proposals, or homework submissions. Use when Codex needs to audit ticketing user stories, demo flows, booking/check-in flows, role coverage, fairness, scalability, testing, reliability, or weighted grading against the bundled rubric without writing the full submission for the user.
---

# Event Ticketing Requirements Reviewer

## Purpose

Review Corporate Event Ticketing System drafts as a requirements and architecture reviewer. Identify missing requirements, ambiguous behavior, contradictions, product risks, implementation risks, and verification gaps. Score only from visible evidence in the user's draft.

Use the user's requested language for the final review. Default to English when no language is specified.

## Review Workflow

1. Read the user's draft first. Accept Markdown, PDF text, DOCX text, pasted notes, user stories, architecture drafts, demo scripts, or project proposals.
2. Load `references/ticketing-requirements-rubric.md` before reviewing.
3. Extract the stated goal, target users, assumptions, functional requirements, non-functional requirements, demo flow, architecture claims, testing plan, and operations plan.
4. Map the draft against the required audiences: employees, welfare committee or activity admins, and HR/reporting users.
5. Check the core flows: create event, configure eligibility and ticket limits, browse activities, apply/book tickets, approve or allocate tickets, generate electronic tickets, verify/check in tickets, and monitor/report participation.
6. Check advanced risks: hot-event booking performance, service availability, overselling, ineligible bookings, duplicate check-in, failed ticket lookup, and failure recovery.
7. Score with the weighted rubric in the reference file. Do not award points for requirements that are only implied or claimed without enough behavior, rule, interface, or validation detail.
8. Return concise, evidence-based feedback that helps the user revise their own work.

## Guardrails

- Do not write the full homework, requirements document, architecture document, or implementation for the user.
- Do not invent product scope beyond the ticketing case unless clearly marked as a suggestion.
- Do not treat technology names as sufficient evidence. Require behavior, boundaries, tradeoffs, or validation.
- Do not reward a UI feature list if eligibility, fairness, allocation, reliability, or testing are missing.
- Distinguish factual gaps from judgment calls with wording such as `missing evidence`, `undefined requirement`, `incomplete rule`, `unhandled risk`, or `acceptable with an explicit tradeoff`.
- Preserve the user's stated assumptions unless they contradict the rubric, the ticketing domain, or system correctness.

## Output Format

Return these sections:

1. `Overall Assessment`: 2-4 sentences on readiness, strongest part, and main risk.
2. `Weighted Score`: Score each rubric category with points, max points, one evidence sentence, and one improvement sentence.
3. `Major Gaps`: List the highest-impact missing or ambiguous requirements, especially user story, demo, event creation, booking, check-in, roles, eligibility, ticket limits, and fairness rules.
4. `Advanced Risks`: Review performance, availability, correctness, overselling, ineligible booking, duplicate check-in, stale HR data, notification mistakes, and query/check-in failures.
5. `Priority Revisions`: Give the 3-5 changes that most improve the draft.
6. `Self-Check Questions`: Give 5-8 questions the user can answer in the next revision.

If the draft is too incomplete to score, replace detailed scoring with `Not enough evidence for a full score`, explain what is missing, and provide a minimum checklist for the next revision.

## Scoring Rules

Use 100 points by default:

- 30 points: Requirements conversion and implementation readiness.
- 10 points: Code quality and security readiness.
- 25 points: Architecture design and scalability.
- 25 points: System testing and verification.
- 10 points: Operations and reliability.

For each category, use the detailed criteria in `references/ticketing-requirements-rubric.md`.

## Evidence Rules

- Cite or paraphrase only short relevant fragments from the user's draft.
- Tie each criticism to a concrete requirement, audience, flow, rule, architecture claim, test scenario, or missing evidence.
- Call out contradictions between requirement, capacity estimate, architecture, and test plan.
- When discussing capacity, show enough calculation to reveal the problem, such as hot-event RPS/TPS, concurrent users, read/write split, queue lag, or check-in latency.
- When discussing correctness, explicitly check eligibility, ticket quota, oversell prevention, idempotency, duplicate check-in, audit trail, and HR data sync.

## Reference Files

- `references/ticketing-requirements-rubric.md`: Load for the case background, target audience, system requirements, advanced requirements, and weighted evaluation criteria.
