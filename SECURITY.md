# Security policy

Pact coordinates humans and AI agents around source code and may eventually
mediate sensitive infrastructure actions. Please report vulnerabilities
privately and avoid disclosing exploitable details in a public issue.

## Supported versions

Pact is currently in early alpha. Security fixes are applied to the latest
released version only.

| Version | Supported |
|---|---|
| Latest release | Yes |
| Older releases | No |

## Report a vulnerability

Use GitHub's private vulnerability reporting for this repository:

<https://github.com/jorgenuanzs/the-pact/security/advisories/new>

Include, when possible:

- the affected version and operating system;
- the deployment topology and relevant configuration with secrets removed;
- reproduction steps or a minimal proof of concept;
- the expected and observed security boundary;
- potential impact;
- any suggested mitigation.

Do not include real API tokens, invitation secrets, private keys, database
dumps, customer data, private repository contents, or agent conversations.

We will acknowledge a valid report as soon as practical, investigate it, and
coordinate disclosure after a fix is available. Please allow a reasonable
remediation period before public disclosure.

## Security scope

Examples of in-scope reports include:

- authentication or authorization bypass;
- cross-project or cross-organization data exposure;
- token, invitation, or secret disclosure;
- unsafe path handling or worktree escape;
- command injection or unintended remote execution;
- sensitive Git content sent to Pact Server contrary to the documented model;
- event, lease, scope, or idempotency invariant violations with security
  impact;
- vulnerabilities in the installer or release verification process.

Pact does not currently claim to sandbox agents or prevent users with direct
filesystem and Git access from bypassing observer mode. Reports that rely only
on those documented limitations may be handled as feature requests rather than
security vulnerabilities.
