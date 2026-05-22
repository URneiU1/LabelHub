# CLAUDE.md

## Project Workflow Notes

After every code change, update project documentation before the final handoff:

- Update `README.md` with what was completed, what still needs improvement, next steps, and verification.
- Update this file when Claude-specific workflow, handoff, or review notes change.
- Keep entries factual and tied to commits or completed work.

## Current Handoff

- Latest completed commit: `0b871d5 fix(security): tighten review upload and payload boundaries`.
- Current focus: S2 implementation after SchemaRenderer runtime and security boundary hardening.
- Next priority: wire SchemaRenderer into the labeler/reviewer flows, then continue Designer editing capability.

## Known Follow-Ups

- Resolve or document the `lottie-web` eval build warning.
- Address web chunk size warning with route-level or vendor chunk splitting when the UI stabilizes.
- Keep permission boundary tests close to every new owner/reviewer/admin endpoint.
