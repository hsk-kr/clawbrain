## Contributor

- [ ] AI agent (specify: ___________)
- [ ] Human

## Summary

Describe the problem and fix in 2-5 bullets:

- Problem:
- Why it matters:
- What changed:
- What did NOT change (scope boundary):

## Change Type

- [ ] Bug fix
- [ ] Feature
- [ ] Refactor
- [ ] Docs
- [ ] Chore / infra

## Scope

- [ ] CLI (`cmd/clawbrain/`)
- [ ] Store (`internal/store/`)
- [ ] Build / Docker
- [ ] Tests
- [ ] Documentation

## Linked Issue

- Closes #
- Related #

## Design Boundary Check

ClawBrain is memory infrastructure. It stores and retrieves vectors. It does not reason.

- Does this PR add reasoning, LLM calls, or embedding generation? (`Yes/No`)
- Does this PR maintain JSON-in, JSON-out contracts? (`Yes/No`)
- Does this PR keep memory and reasoning cleanly separated? (`Yes/No`)

If any answer violates the [design philosophy](AGENTS.md#design-philosophy), explain why.

## Changes

List what changed and where:

-
-

## Verification

### Environment

- OS:
- Qdrant version:

### How it was tested

Describe what was tested and how. Include commands run:

```bash
# e.g.
go test ./... -v
clawbrain check
```

### Evidence

Attach at least one:

- [ ] Test output (before/after)
- [ ] CLI JSON output showing correct behavior
- [ ] Log snippets

## Compatibility

- Backward compatible? (`Yes/No`)
- Existing CLI flags or JSON output changed? (`Yes/No`)
- If yes, describe the migration path:
