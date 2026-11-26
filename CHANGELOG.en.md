# Changelog

[简体中文](CHANGELOG.md) | **English**

Versions follow [Semantic Versioning](https://semver.org/). Each release groups the changes users will notice; the newest release is at the top.

## v0.5.1

### Added

- Host **Network** page: when Caddy or Nginx is installed but not running, a **Start** button appears next to the status (previewed and approved as usual). Previously the page only said “Not running” and you had to run the command yourself.

### Improved

- Adding or removing a site now also starts Caddy or Nginx if it is not running; previously that step failed and left behind a site that could not be reached.
- When starting or reloading Caddy or Nginx fails, the request output includes the service's recent log, so the cause is visible right away (usually another program holding port 80 or 443).
- On the **Network** page, after you submit an install of Caddy, Nginx or Fail2ban, or enable the firewall, the button turns into “Awaiting approval” / “Installing…” and links back to that request, so it cannot be submitted twice.
- The description of **Listen port** under **Settings → Panel** now makes clear that it is the port Hostower itself listens on; behind a reverse proxy, the port you use in the browser is the one in the public URL.
- FAQ additions: what to check when an AI assistant cannot connect over MCP (an untrusted self-signed certificate, Codex not seeing the token environment variable), and what to do when the Sites card says “Not running”.

### Fixed

- When a command finished very quickly, the request stayed at “Running” on the page until a refresh.

## v0.5.0

### Added

- **MCP access**: AI assistants such as Claude Code and Codex can connect to the panel's `/mcp` over MCP (Model Context Protocol) to list hosts and metrics, estimate a command's level, submit commands, and check or cancel the requests they submitted. Their commands are classified as usual: read-only ones run directly, the rest wait for a person to approve them in the web UI. The AI cannot approve, and there are no terminal, file, credential or settings tools. The owner issues tokens under **Settings → Security → MCP access**; a token is shown once, can have an expiry and can be revoked at any time. Every call is audited with the entry `mcp`.
- **Resource alerts**: a notification is sent when any mount point is above 90% for 5 minutes, memory is above 90% for 5 minutes, or CPU is above 90% for 10 minutes, and a recovery notice once it has stayed below the recovery line for 5 minutes, so it does not flap around the threshold. It only uses the samples already collected and runs nothing new on the host; existing notification channels subscribe to the three new events automatically, and alerts are not resent after a panel restart.

### Improved

- The script downloaded on first visit (and the first visit after each upgrade) dropped from about 2.5 MB to about 250 KB: only the current language's dictionary is downloaded, and static files are sent compressed.
- The "Runs, last 24h" and "Pending approval" cards on the overview now state the outcome (All succeeded, N failed, All clear) instead of following the number with "No failures" / "Nothing waiting".
- The egress column of the environment table labels both IPv4 and IPv6, with the IP on hover; previously only v6 was labeled, which looked as if hosts had IPv6 only.

### Fixed

- A thin line briefly appeared when switching to the Snippets page.

## v0.4.3

### Added

- Apps with a web panel under Apps → Installed (OpenList, 3x-ui, Uptime Kuma, Portainer, LiteLLM, Open WebUI, New API, n8n) get an **Open panel** button that opens it in a new tab. The address is built from the instance's current listen address and port, so it follows changes made in **Edit settings**; if the app only listens on 127.0.0.1 you are asked to set a panel address first.
- The **Panel address** next to it can be set per instance: override the port and path (for example the panel port and path changed inside 3x-ui), enter a full address (sites on the Network page that already reverse-proxy to that port are listed and fill in with one click), or go through an SSH tunnel (the page shows an `ssh -N -L` command and the button opens the local port). Only the owner can change it; it does not touch the host, every change is audited, and the address cannot contain a username, password, query or fragment.

### Fixed

- After a page sat open for a long time or the computer woke from sleep, the traffic card sometimes showed "Cannot reach the tower" until the page was reloaded.

## v0.4.2

### Improved

- Hosts on the collaboration map are now circles like the control tower: the arc of the outer load ring is CPU usage (yellow above 70%, red above 90%), the small server lights inside show online, running or offline, and the line under the name spells out CPU or "Waiting for approval" / "Offline"; assistants sit on the right of the circle and gateways on the left, and links stop outside the ring.
- The collaboration map is shown at most at its natural size and centered, so icons and text no longer grow on wide screens.

### Fixed

- The task and summary boxes were only one line tall with the text squeezed together; they are multi-line again.
- The letter inside an assistant's round badge sat off-center with some system fonts.

## v0.4.1

### Improved

- After approving or denying, you are taken back to the page the operation was submitted from (host details, containers, apps, network, files, Agents) instead of having to find it again from the overview; approving from the Approvals list returns to the list so you can approve the next one. Run, Snippets and opening a terminal still stay on the request page to show the output.
- Back on the original page, the container list, installed apps, network settings and file list refresh by themselves when the related request finishes; updating the enhanced collector first shows "Installing…" and then the new version.

## v0.4.0

### Added

- Agent collaboration (a new page in the sidebar):
  - Dispatch a task: hand the same task to Claude Code, Codex CLI or Gemini CLI on several hosts, each working on its own host. Read-only by default; you can allow edits inside the working directory only. Dispatching is one request and runs only after approval.
  - Compare results side by side, then "Summarize": the outputs of all hosts are combined and given to the assistant on one host to compare and summarize (also a request).
  - Collaboration map: a live picture of the tower and the hosts. Operations fly along the lines like small packets (colored by agent task, file distribution, app install and other commands), busy assistants bob with a "…" bubble, flowing links show which LiteLLM / New API gateway an assistant calls models through, and hosts waiting for approval flash a question mark. Animations stop when the system asks for reduced motion.
  - Assistants and gateways per host: which assistants are installed, which model provider they use, which gateway they go through and how many tasks they ran recently.
- Risk levels: running an AI coding assistant normally is a change (L1); skipping permission prompts or the sandbox (`--dangerously-skip-permissions`, `--yolo`, `--sandbox danger-full-access` and so on), or arguments that cannot be fully seen, are high risk (L2).
- Environment check:
  - Egress IP is now shown in two columns, IPv4 and IPv6, each with its own ownership, type, risk score and proxy flag (before, hosts with IPv6 only showed the IPv6 egress).
  - New "AI services: region and anti-abuse": which region ChatGPT, Claude and Google put the host in and whether it is within their usual service regions; whether the ChatGPT website demands a CAPTCHA, whether Google search treats the host as unusual traffic, and whether the IP location matches the region the AI services see. The Apps → Environment table gains a column for these signals.

### Fixed

- Risk levels: prefixing a command with `command` / `builtin` (e.g. `command rm -rf /`) turned L3 into L1, and paths such as `/etc/./shadow` or `/proc/self/root/etc/shadow` slipped past the private-key and credential rules as read-only. Both are now classified as written.
- A host that accepted the connection but never finished the SSH handshake left requests "running" forever; file reads and writes did not stop on timeout or when the browser disconnected; a silently dropped network kept being reused as a dead connection.
- Running on many hosts re-saved the entire output every time one host finished, slowing down other commands running at the same time.
- Uploaded files are re-checked against their content digest before running (previously only the size).
- When the image registry or npm was unreachable, opening the Apps page waited for a timeout per app; automatic update requests could resubmit a version that had been denied.
- Web UI: the Agents page went blank when a snippet's name started with "agent"; the Installed list failed with more than 50 hosts; data from the previous session stayed in memory after signing out; a long-open page kept growing its request list and output cache; every output chunk re-rendered every output box on the page.

### Improved

- README screenshots now show the current interface.

## v0.3.0

### Added

- File sync (a new page in the sidebar):
  - Distribute to hosts: drag files onto the page, pick a target directory and hosts (all, filtered by tag, or one by one), up to 20 files at a time. Each file is one request and is written to the same directory on the chosen hosts after approval; risk levels, approval and audit work the same as for typed commands.
  - Collect from hosts: find files on several hosts by directory and file-name pattern (wildcards such as `*.log`), preview the list, then download them as one archive laid out as "host name/file path". Finding is a read-only query, every downloaded file is audited, and private keys and credential files still cannot be read.

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
