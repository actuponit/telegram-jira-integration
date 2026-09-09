# 02 — App Context content file

**What to build:** one static file in this repo describing the Mela app —
what the model is given so it can recognize product shorthand and pick the
right `App` label.

**Blocked by:** none

**Status:** done

## Scope

- One new file (location: pick a sensible spot under the repo, e.g.
  `internal/adapters/gemini/appcontext.md` or similar — ticket 05 loads it,
  so put it somewhere that adapter can embed/read from) covering the Mela app
  only: what the product is, its feature list with a one-line description
  each, and the platform/third-party services that appear in reports
  (Flutter, BLoC, Onfido, Stripe, Plaid and similar).
- Roughly six hundred words, per the spec.
- Hard boundary: **what exists, never how it works**. Feature names and
  vendor names are in. Internal architecture, file layout, and routing logic
  are out — that's the material that tips the model from classifying into
  diagnosing.
- Merchant App and Backend get no context payload — do not draft one. Their
  Candidates are drafted from the conversation alone, per spec, and quality
  is knowingly lower for them.
- Seed content from the Mela app's own feature index if accessible from this
  session's working directories; otherwise draft from what's already known
  about the product and flag for human review in `## Notes`.

## Done when

- The file exists, is under the repo (not generated at runtime), and is
  loadable as a plain string by ticket 05's adapter.
- Word count is roughly 600, not a sprawling architecture doc.
- No sentence describes *how* a feature is implemented (no file paths, no
  internal routing, no code-level detail) — only that it exists.

## Open

Exact file path/format (`.md` vs `.txt` vs Go string const) isn't specified
in the spec. Whoever picks up ticket 05 should confirm the format matches
what `SystemInstruction` construction wants.

## Notes

File placed at `internal/adapters/gemini/appcontext.md` (586 words).
Feature list and vendor list seeded from `mela_fi_ui`'s
`lib/presentation/features/` directory names and `pubspec.yaml`
dependencies (this session's working directories included that repo).
Feature one-liners are inferred from folder names, not read line-by-line
from a maintained index — flagging for human review in case any
description drifts from what the feature actually does today.
