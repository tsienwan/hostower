# Changelog

[简体中文](CHANGELOG.md) | **English**

Versions follow [Semantic Versioning](https://semver.org/). Each release groups the changes users will notice; the newest release is at the top.

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
