# Onboarding persona × condition × goal matrix

Goal: pressure-test new-user onboarding. A blind agent simulates each persona reaching a
first-step goal using only user-facing docs, in the persona's container.

| ID | Container | Persona (skill) | Starting condition | First-step goal | Primarily probes |
|----|-----------|-----------------|--------------------|-----------------|------------------|
| S-A | `ptux-sa` | Impatient first-timer (low) | chromium present | open example.com, read title | happy path + IDPI block for a non-technical user |
| S-B | `ptux-sb` | Backend dev (high) | **no browser** | scrape a page's heading | no-browser cold start, scraping flow |
| S-C | `ptux-sc` | Privacy researcher (med) | chromium present | browse the **public** web | IDPI allowlist / guards journey, security legibility |
| S-D | `ptux-sd` | DevOps / CI (high) | **no browser, minimal** | scriptable title (JSON/exit codes, restart) | scriptability, restart cleanliness |
| S-E | `ptux-se` | Mis-set config (med) | chromium + **bogus `browser.binary`** | get nav working again | diagnosability (doctor), recovery |
| S-F | `ptux-sf` | API integrator (high) | chromium + curl | open a page via the **HTTP API** | API discoverability, auth/token UX |

## Per-scenario record (each agent returns)
Chronological friction log → for each step: command, key output (quoted), user
interpretation, could-proceed-from-the-messaging-alone (yes/partial/no), one-line fix.
Then: goal achieved? top friction points; onboarding verdict for that persona.

## Synthesis (after a run)
- Cross-persona friction table (recurring vs persona-specific).
- Ranked onboarding improvements scoped to what's fixable in this repo.
- Validation that prior fixes still hold under persona pressure.
