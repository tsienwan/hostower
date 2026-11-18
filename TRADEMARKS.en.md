# Trademarks

[简体中文](TRADEMARKS.md) | **English**

Hostower's **code** is open source under the [GNU AGPL-3.0](LICENSE); its **name and logo** are not covered by that license — they are the project's trademarks.
These are separate matters: the AGPL gives you the freedom to use, modify and redistribute the code, but not the right to use the name "Hostower" or its logo to represent your own version.

The word "Hostower" and the Hostower logo are trademarks used by the author of Hostower (™, not yet registered).

## Allowed without permission

- Using and deploying official Hostower releases as they are (including inside a company or for clients), and calling it Hostower.
- Mentioning Hostower in articles, tutorials, reviews and comparisons, with a link to the official repository.
- Saying your project is "based on Hostower", "compatible with Hostower", or "a plugin or rule pack for Hostower", as long as nobody is led to think it is official or endorsed.
- Modifying the code to contribute improvements (pull requests) and keeping the Hostower name in development branches.

## Requires a different name and logo

- **Redistributing a modified version**: if you distribute modified code (binaries, images, packages) or offer it as a service, use a name and logo that cannot be confused with Hostower, and remove the Hostower branding from the interface.
  You may say "based on Hostower" in the description.
- **Commercial products or services**: don't use "Hostower" or a confusingly similar name in a product name, company name, domain or app-store listing, and don't use the Hostower logo or an imitation of it.
- **Don't pass off a build as official**: don't call a build "official" or a "release" unless it was signed with the official release key, and don't forge the build-signature indicator in the interface.

The AGPL's obligations don't go away with a new name: a modified version offered over a network must still provide its users with the corresponding source code.

## How to tell an official build

Official binaries carry a build seal at the end of the file, signed with the release key:

- The interface shows "release signature verified" under **Settings → About → Build**, along with the release public key fingerprint;
- You can also verify offline on the host, without relying on anything compiled into the file:

  ```bash
  hostower verify <official release public key> /usr/local/bin/hostower
  ```

The interface indicator alone is not enough: someone who changes the code can forge it too. **The public key fingerprint is what counts** — it must exactly match the official fingerprint published below.

Official release public key fingerprint: **no official release has been published yet**. Before the first release, generate the release key with `go run ./tools/relsign gen`, put the printed fingerprint here, and publish it on the official website or releases page at the same time.

## Third-party trademarks

Names and logos of other software and services in the app catalog (for example Redis, PostgreSQL, Claude Code, DeepSeek) belong to their respective owners.
They are used here only to identify the app and do not imply endorsement or sponsorship of this project. Logo sources are listed in [internal/apps/catalog/icons/README.md](internal/apps/catalog/icons/README.md).

If you have questions about using the name or logo, please open an issue first.
