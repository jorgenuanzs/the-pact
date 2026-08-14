# ADR-0013 — Structured Handoffs and verifiable Context Packs

**Status:** accepted
**Date:** August 14, 2026

## Context

Pact can already represent live sessions, coordinated intentions, isolated Git
worktrees, scope leases, and durable Workspace knowledge. A collaborator still
needs a safe way to relinquish work and another collaborator needs a bounded
context object that does not depend on inheriting the first collaborator's
private conversation.

A chat transcript is not an appropriate handoff contract. It mixes discarded
hypotheses with accepted facts, has no stable lifecycle, and is difficult to
authorize or verify. Automatically handing a local worktree from one machine to
another would also be unsafe: filesystem paths are machine-specific and the
recipient may not possess the same Git objects or uncommitted changes.

## Decision

Pact introduces two related but separate concepts.

### Handoff

A Handoff is a durable offer attached to one coordinated intent. It records:

- a self-contained summary;
- completed and remaining work;
- blockers and recommended next steps;
- validations and their status;
- links to durable Workspace Records;
- the offering session and actor, accepting session and actor, version, and
  expiry.

Its lifecycle is `offered`, `accepted`, `withdrawn`, or `expired`. Only one
unexpired offer may exist for an intent. The responsible agent offers it; a
different live agent accepts it. Commands are idempotent and transitions use
optimistic version control.

Acceptance means **receipt acknowledged**. It does not transfer a filesystem
path, worktree, scope lease, session, responsibility, or uncommitted Git state.
The safe continuation flow is to close or release the original work and let the
recipient start a new coordinated intent with its own scopes and worktree.

Reading expired offers is side-effect free. When a later command materializes
an expiration, Pact emits `pact.handoff.expired.v1`; offers and responses also
produce immutable project events.

### Context Pack

A Context Pack is an immutable, intent-specific snapshot assembled
deterministically from:

- project and Workspace identity;
- the selected intent and currently relevant work;
- accepted or otherwise relevant Workspace knowledge;
- handoffs for the intent;
- the canonical Git revision and project event cursor.

The first implementation supports `implementation`, `handoff`, `review`,
`onboarding`, `meeting`, `incident`, and `deployment` packs. It records eventual
consistency explicitly, expires after a short caller-selected TTL, stores a
SHA-256 fingerprint of its source object, and verifies the hash of the stored
payload when read.

Context Packs omit machine-local worktree paths and repository credentials.
They contain structured state, not prompts, private conversations, or an LLM
summary. Future retrieval and synthesis can build on the same envelope without
changing these integrity and provenance requirements.

## Surfaces

The HTTP API exposes project-scoped list, offer, response, compile, and get
operations. Project roles apply before the domain command is evaluated.

MCP adds:

```text
pact.list_handoffs
pact.offer_handoff
pact.update_handoff
pact.compile_context_pack
pact.get_context_pack
```

The backoffice displays handoff state, participants, remaining work, blockers,
next steps, validations, version, and expiry. It remains read-only.

## Consequences

- Agents can exchange work state without sharing their conversations.
- A Context Pack can be cached, compared, audited, and rejected after expiry.
- Receipt is not confused with ownership of local code or permissions.
- The first pack compiler is deterministic and bounded, but does not yet offer
  hybrid retrieval, token budgets, per-record authorization, deltas, or
  automatic synthesis.
