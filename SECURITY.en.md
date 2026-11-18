# Security policy

[简体中文](SECURITY.md) | **English**

Hostower holds the keys to your hosts. Please report security issues **privately**, not in a public issue.

## How to report

Use the hosting platform's private vulnerability reporting (the repository's Security → Report a vulnerability).
Include the affected version, steps to reproduce, and the impact you believe it has.

## Response times

- Acknowledgement within 3 business days.
- An initial assessment and plan within 10 business days.
- Public credit after the fix is released (if you wish).

## Scope

We pay particular attention to:

- Any way to bypass classification, approval or audit (including letting L3 run, or letting an AI or token complete an approval).
- Under-classification: a command whose real impact is higher than the level it receives. Please include the command; we will add it to the test table.
- Credentials appearing in logs, the audit log, API responses or model input.
- Authentication and session problems, cross-site request forgery, cross-site scripting.
- Silent changes to the audit hash chain that verification does not catch.

Until it has passed an external security review, we don't recommend exposing Hostower to the public internet.
