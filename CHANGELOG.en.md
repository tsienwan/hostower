# Changelog

[简体中文](CHANGELOG.md) | **English**

Versions follow [Semantic Versioning](https://semver.org/). Each release groups the changes users will notice; the newest release is at the top.

## v0.2.0

### Added

- Getting-started guide for apps: after an app is installed, click the rocket icon under Apps → Installed to see its address (clickable), what the initial account is and the next few steps, without logging in to the host to dig through logs. A successful install request links to it as well. Apps that only listen on the host itself say that a reverse proxy is needed.
- Read the initial account: 3x-ui generates a random username and password on first start and prints them once in its log; the guide can now read that line for you (a single read-only query; the result is shown, not stored).
- New apps in the catalog: OpenList, Open WebUI, LiteLLM, New API, Ollama, n8n and Gemini CLI, plus a new Storage category. OpenList's admin password and LiteLLM's master key are set from app secrets at install time. LiteLLM and New API come with their own dashboards for per-key token usage.
- Environment check per host: a new Environment tab on the host page.
  - Egress IP: address, location, operator, type (data center, residential, business and so on), whether it is flagged as a proxy, a risk score, and whether there is IPv6 egress.
  - LLM APIs: whether OpenAI, Anthropic (Claude), Google Gemini, xAI and DeepSeek can be used (available, restricted, unreachable).
  - Websites and streaming: whether Google, YouTube, Netflix, Disney+, Prime Video, Spotify, TikTok, Facebook, Instagram, X, Telegram, Discord, Reddit, the ChatGPT and Gemini web apps, GitHub, Docker Hub, npm and Wikipedia open; YouTube and Netflix show which region the host lands in, and Netflix also tells whether non-original titles are available.
  - Which AI command-line tools (Claude Code, Codex CLI, Gemini CLI and others, with version and sign-in state) and runtimes are installed; missing ones link straight to installation.
- Apps → Environment: one table for all hosts, showing each host's egress type and risk score, which model APIs it can use, which region common sites put it in, and which AI command-line tools it has.

- Install progress: the request page of an app install shows a progress track with the current step (prepare, download, create, start, done) and, while images download, how many layers are finished; a running request has a moving highlight on top. Animations are off when the system asks for reduced motion.

### Improved

- On phones the bottom bar keeps only Overview, Run, Approvals and Apps; the other pages (Activity, Containers, Speed test, Snippets, Credentials, Audit, Settings) move into More.
- Operations that need approval (installing apps, opening ports, changing sites and so on) take you to the request page after submitting: the request and approvals pages now have a "Back to the previous page" button, and a finished request offers it too, so you no longer depend on the browser's back button.
- The Installed list shows an Open link for apps that are reachable from outside; catalog cards link to the official documentation.
- Corrected the 3x-ui notes: the panel listens on port 2053 by default and the initial account is not fixed.

## v0.1.7

### Fixes

- With the page left open, after the network dropped and came back the traffic card and metric history could keep showing "Cannot reach the tower" for up to a minute; they now reload as soon as the live connection is restored.

## v0.1.6

### Fixes

- After a network drop or the computer sleeping, a host's traffic card could keep showing "Could not read traffic: Cannot reach the tower" for a long time; the traffic card, traffic ranking, this-machine card and metric history now reload as soon as the tab becomes visible or the network comes back.

## v0.1.5

### Fixes

- After the page sat idle for a long time (computer asleep, network dropped for a while) it could stay on "Reconnecting" and stack up "Cannot reach the tower" toasts; it now reconnects as soon as the network comes back or the tab becomes visible, returns to the sign-in page if the session has expired, and no longer repeats the same toast.
- On a host's page, the network and disk I/O cards' headline number, legend and min/avg/max did not agree: the legend now shows the latest sample like the headline, and min/avg/max are computed on the combined total (previously only inbound or read).

## v0.1.4

### Fixes

- After installing or starting Fail2ban from the Network tab, the final status check could fail with "Failed to access socket path" and mark the operation as failed even though it had succeeded; it now waits until Fail2ban responds before checking.

## v0.1.3

### New

- Monthly traffic quota per host: set the allowance (GB/TB), billing mode (outbound only, inbound only, sum, or the larger of the two), monthly reset day and an optional single interface, and calibrate against the provider's panel. The host page shows usage this cycle, days left, projected end-of-cycle usage and daily/monthly bar charts; the overview shows a progress bar and a traffic ranking.
- Traffic alerts: one notification through the existing channels when usage reaches a set percentage, and one when the current pace is projected to exceed the quota; each fires at most once per level per cycle.
- New read-only chat bot command `/traffic`.

### Improvements

- Network rates and monthly traffic count only uplink interfaces: traffic on tunnels, bridges, VLAN sub-interfaces and bond members is no longer counted twice.
- In the dark theme, tabs and segmented buttons now look like a macOS segmented control.

### Fixes

- When the service running Hostower forbids privilege escalation (systemd NoNewPrivileges), removing an app together with its data failed because files written by the container could not be deleted; a temporary container now deletes them.
- After a page sat idle for a long time, the browser could reuse a connection the network had silently dropped, leaving parts of the page blank; read-only requests now retry once on a network error.

## v0.1.2

### New

- The Network tab of a host can manage Nginx and Caddy reverse-proxy sites: three HTTPS modes, certificate list and renewal, and access and error logs; the configuration is validated before it is written.
- The same tab installs and configures Fail2ban: enable SSH protection, set ban time and retry count, and view and unban blocked IPs. Because Fail2ban controls the firewall, every change other than viewing needs approval with on-the-spot verification in the web UI.

### Improvements

- "Listening ports" is now a table grouped by source: one row for host processes and one per container. Container ports that bypass the firewall are flagged once, with the advice to bind them to 127.0.0.1.

## v0.1.1

### Fixes

- When the sign-in session expires, the page no longer sits on "Reconnecting…": a dropped live connection now checks the session and returns to the sign-in page if it has expired.
- If refreshing a host's history charts fails (network drop, SSH tunnel closed, expired session), the charts are no longer wiped to "Not enough history"; the data already shown stays and the reason is displayed, and the history is fetched again as soon as the tab is visible or the network is back.
- Failures to write metric history to the database are now logged instead of being dropped silently.

## v0.1.0

The first release.

### Security kernel

- Every entry point goes through one path: classify → approve → execute → audit. Levels only ever go up; nobody can allow L3.
- The rule engine classifies each command from the shell syntax tree, and its behavior is pinned by a test table; rule packs can only tighten.
- Approvals are bound to a digest of what will run; L2 needs on-the-spot Passkey or authenticator verification in the web UI; when an AI submits a request, a person must approve it.
- The audit log uses a hash chain and database triggers against tampering, keeps a JSONL file copy and can be verified offline; optional archive-then-prune.
- Credentials are envelope-encrypted, and only `ref:<name>` ever appears elsewhere.

### Hosts and monitoring

- Hosts are managed over SSH with nothing to install; a built-in local host; add, edit, delete and reconnect hosts from the UI.
- One long-lived SSH connection per host streams a sample every 5 seconds, including per-container usage; history is kept on disk with adjustable retention.
- Optional enhanced collector: one-click install from the panel (through approval), adds a process ranking, and takes effect within seconds of installing or uninstalling.

### Operations

- Run commands with streaming output; batches, rolling runs, dry runs, stop on error; parameterized Snippets.
- Interactive terminal: approved as L2 and fully recorded.
- Browse, search, preview (images, PDF, Markdown, CSV, Office documents), edit, upload and download files.

### Containers and apps

- Manage containers and images, deploy or update by image, registry logins, image archive import; images with fixed version tags show newer versions within the same major version.
- App catalog: Docker Compose apps and npm CLI tools; AI coding tools can be configured with a model provider (DeepSeek Harness supports third-party providers and custom model endpoints).
- Hosts without Docker or Docker Compose can install it from the UI (from Docker's official repositories, per OS).

### Other

- Sign-in: Passkeys, passwords (with optional authenticator codes), OIDC single sign-on.
- Notifications: Telegram, Feishu/Lark, Discord, email; Telegram direct-message commands.
- Speed test: latency, download, upload, with optional traceroutes; guesses the carrier route from backbone prefixes and autonomous systems (China Telecom CN2 GIA/GT, 163, CTGNet; China Unicom 9929/4837; China Mobile CMIN2/CMI/CMNET), and marks the long-haul segment and hop locations that may be wrong.
- Signed self-update; three deployment methods: default image, host-mode image, binary.
- The interface comes in seven languages with light and dark themes.
