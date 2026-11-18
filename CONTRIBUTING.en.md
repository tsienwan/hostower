# Contributing

[简体中文](CONTRIBUTING.md) | **English**

Thanks for taking part. Before you start, read [AGENTS.md](AGENTS.md) (Chinese): it holds the kernel's hard rules, which apply to people and AI assistants alike.

## License

Contributions to this repository are licensed under [AGPL-3.0](LICENSE), like the rest of the project; by opening a pull request you confirm you have the right to license your contribution this way.
Rules for using the project name and logo are in [TRADEMARKS.en.md](TRADEMARKS.en.md).

## Development setup

- Go 1.26+, Node 24+.
- `git config core.hooksPath .githooks` enables the pre-commit checks.

## Before you commit

```bash
gofmt -l .                 # should print nothing
go vet ./...
go test ./...
go run ./tools/hygiene
go run ./tools/i18n        # missing translations
cd web && npm run build
```

## Working with AI coding assistants

The project rules live only in [AGENTS.md](AGENTS.md). OpenAI Codex and DeepSeek Harness read it directly; Claude Code pulls it in through [CLAUDE.md](CLAUDE.md).
Don't write a separate copy of the rules for any one tool; put personal local additions in `AGENTS.local.md`, `CLAUDE.local.md` or `AGENTS.override.md`, which stay out of the repository.

## Versions and releases

- Versions follow Semantic Versioning, starting at `v0.1.0`.
- Before a release, add a section at the top of [CHANGELOG.md](CHANGELOG.md) and [CHANGELOG.en.md](CHANGELOG.en.md), and update `web/package.json`, the image tags under `deploy/`, and the current version mentioned in the documentation.
- Push a `vX.Y.Z` tag; the [release workflow](.github/workflows/release.yml) builds, signs and pushes the images.
  Before the first release, run `go run ./tools/relsign gen` to create a release key pair: put the public key in the repository variable `HOSTOWER_UPDATE_KEY` and the private key in the secret `HOSTOWER_RELEASE_KEY`.

## Conventions

- **Change the test table before a classification rule** (`internal/kernel/risk/testdata/table.txt`), then the code.
- **The security kernel takes no extension points**: no switches or hooks for highest-level classification, L3, the approval state machine or audit writes.
- **Nothing from a real environment goes into the repository**: examples use reserved address ranges and `example.com` only.
- One change does one thing; the description explains *why*, not just *what*.
- Code comments, UI text and error messages are written in Simplified Chinese (the UI translates them); identifiers are in English.
- README and the FAQ exist in seven languages, the other guides in Chinese and English (`*.en.md` is English); when you change one, update the others. Screenshots live in `docs/images/screenshots/`; retake them when the UI changes.
- Don't open public issues for security problems; see [SECURITY.en.md](SECURITY.en.md).
