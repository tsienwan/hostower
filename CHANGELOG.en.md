# Changelog

[简体中文](CHANGELOG.md) | **English**

Versions follow [Semantic Versioning](https://semver.org/). Each release groups the changes users will notice; the newest release is at the top.

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
