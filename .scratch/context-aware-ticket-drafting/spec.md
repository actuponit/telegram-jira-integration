Status: ready-for-agent

# Context-Aware Multi-Ticket Drafting — Spec

## Problem Statement

The bot turns one Ticket Request into exactly one Issue, drafted from the source
message alone with no knowledge of the products being discussed. Three things
go wrong as a result.

A reporter often describes several unrelated problems in one message. Today
those collapse into a single Issue that nobody can close, because closing it
would mean fixing three things at once.

A reporter frequently writes in product shorthand — "the Onfido screen froze",
"the OTC agent list is empty", "Plaid didn't connect". Gemini has never heard of
these, so it paraphrases them into a title no engineer recognises, and the
Issue has to be re-read against the original chat to be actionable.

And sometimes a message simply isn't a ticket yet — "the app is broken again".
The bot files it anyway, producing an Issue that costs more to triage than it
saves.

## Solution

One Ticket Request can now produce several independent Issues, and the bot
knows enough about the products to draft them well.

Gemini receives a short, static **App Context** describing the Mela app — what
it is, the features it has, and the third-party services it uses — alongside
the gathered messages. It returns one or more **Candidates**, each a Draft plus
a status. Candidates are split only where each piece could ship on its own and
someone would notice; anything that is merely a step toward another Candidate
stays merged into it.

Candidates it can draft are created immediately, exactly as today — fire and
forget, links in the chat reply. A Candidate it genuinely cannot draft — where
it cannot tell what the reporter wants changed — is not filed. Instead the bot
asks, once, in plain language. Its question message restates what it already
understood; the reporter replies to that message in the chat, and the bot
drafts the Issue from its own summary plus the answer. No confirmation step is
introduced anywhere: the bot never asks "is this right?", only "what did you
mean?".

## User Stories

1. As a Telegram group member, I want one message describing three unrelated problems to become three Issues, so that each can be assigned, worked and closed independently.
2. As a Telegram group member, I want the bot to keep related symptoms in one Issue, so that my board isn't flooded with duplicates of the same underlying problem.
3. As a Telegram group member, I want Issues that never block one another, so that no ticket has to wait on another ticket that came from the same message.
4. As a Telegram group member, I want the bot to default to a single Issue unless there is clear evidence of a second independent problem, so that a long rant doesn't shatter into fragments.
5. As a Telegram group member, I want a hard ceiling on how many Issues one message can produce, so that a runaway split can never flood the board.
6. As a non-technical reporter, I want to describe a problem in the words I'd use with a customer, so that I don't have to learn engineering vocabulary to file a ticket.
7. As a non-technical reporter, I want the bot to recognise product names like Onfido, Plaid and Stripe when I mention them, so that the Issue title uses the words my team already uses.
8. As an engineer picking up a ticket, I want the title to name a feature area I recognise, so that I can tell at a glance whether it's mine.
9. As an engineer picking up a ticket, I want the description to stay factual about what was reported, so that I'm not misled by a guessed root cause.
10. As an engineer, I want the bot never to assert which file, screen or service is at fault, so that a confident-sounding guess doesn't send me down the wrong path.
11. As a Telegram group member, I want each Issue labelled with the app it was observed in, so that the board can be filtered by product.
12. As a Telegram group member, I want a purely server-side problem labelled as such, so that it routes to the backend team rather than a mobile engineer.
13. As a Telegram group member, I want the bot to label by where the problem was *observed* unless it is clearly backend-only, so that the label stays a fact rather than a diagnosis.
14. As a reporter, I want the bot to file a ticket even when I've left out repro steps, my device, or the exact screen, so that missing detail never blocks a report.
15. As a reporter, I want the bot to note what was missing inside the Issue, so that whoever triages knows what to chase.
16. As a reporter, I want the bot to ask me a question only when it genuinely cannot tell what I want changed, so that I'm not interrogated over details that don't matter.
17. As a busy reporter, I want at most three questions, all in one message, so that answering is a single quick interruption.
18. As a busy reporter, I want to be asked at most once, so that filing a ticket never turns into a back-and-forth conversation.
19. As a non-technical reporter, I want the questions phrased in everyday language, so that I can answer without knowing how the system works.
20. As a reporter, I want the bot's question to show me what it already understood, so that I can correct it in the same reply.
21. As a reporter, I want to answer by replying normally in the chat, so that I don't have to remember a command.
22. As a reporter, I want my answer to produce the Issue immediately, so that one reply is all it costs me.
23. As a reporter, I want the bot to file whatever it can from my answer even if I was vague again, so that a second round of questions never happens.
24. As a reporter, I want a stale question to expire quietly after a day, so that answering an old message doesn't file a surprise ticket.
25. As a Telegram group member, I want the good tickets from a message created even when one part of it needed clarification, so that a single vague fragment never holds up the rest.
26. As a Telegram group member, I want one grouped reply listing everything that happened, so that I can see created Issues, failures and open questions in one place.
27. As a Telegram group member, I want each created Issue's link in that reply, so that I can jump straight to any of them.
28. As a Telegram group member, I want to be told explicitly which Issues failed to be created, so that I know exactly what to retry by hand.
29. As a Telegram group member, I want a failure on one Issue not to cancel the others, so that partial success is still success.
30. As a Telegram group member, I want `@assignee` applied to every Issue from that message, so that I only have to say who owns it once.
31. As a Telegram group member, I want the screenshot attached to every Issue from that message, so that evidence isn't lost on whichever ticket happens to need it.
32. As a Telegram group member, I want an unresolved `@assignee` to still create every Issue, so that a typo'd handle never blocks anything.
33. As a Telegram group member, I want the bot to ignore ordinary chat, so that adding it to a group doesn't make it noisy.
34. As a Telegram group member, I want the bot to ignore a reply to one of its "Created" confirmations, so that its existing `/status` behaviour is untouched.
35. As the person maintaining the bot, I want the App Context to live in one file in this repo, so that I don't have to keep three product repositories in sync.
36. As the person maintaining the bot, I want the App Context capped at what exists rather than how it works, so that it can't push the model toward diagnosis.
37. As the person maintaining the bot, I want no database, so that the deployment stays a single Cloud Run service.
38. As the person maintaining the bot, I want every stage of a multi-Issue run logged with the Telegram message ID, so that a partial failure is diagnosable from Cloud Logging alone.
39. As the person maintaining the bot, I want one Gemini call per Ticket Request and one per clarification answer, so that request-metered quota stays predictable.

## Implementation Decisions

### Domain

- New `Candidate` type: a `Draft` plus a `CandidateStatus` (`Ready` or `NeedsClarification`) and its `OpenQuestions`. A Candidate is a proposed Issue that has not been created yet — the term sits between Draft and Issue in the glossary.
- New `App` type with exactly three values: `Mela App`, `Merchant App`, `Backend`. It becomes a field on `Draft` and is written to the Issue as a label. It is single-valued and mutually exclusive.
- New `DraftSet`: the model's `SplitReasoning` plus its `Candidates`. Returned whole by the drafter so the reasoning can be logged.
- `Priority` and `IssueType` are unchanged.

### Ports

- `TicketDrafter.Draft` changes its return from `domain.Draft` to `domain.DraftSet`. This is the breaking change that ripples outward.
- `TicketDrafter` gains a second method for the clarification path, taking the bot's own prior summary text and the reporter's answer, returning a single `Draft`. It is a separate method rather than a flag on `Draft` because its input is a summary and an answer, not a message thread.
- `IssueTracker` and `AssigneeResolver` are unchanged.
- No new port is introduced for the App Context. The use case does not need app knowledge; only the drafting adapter does, so it is adapter configuration injected at wiring time, not a port.

### Use cases

- `CreateTicketFromMessage` becomes plural in behaviour and returns a result carrying: the Issues created, the Candidates that failed to create, and at most one clarification request. It iterates Candidates in order, creating each `Ready` one; a failure on one is recorded and the loop continues. There is no rollback.
- A second use case handles the clarification answer. It takes the bot's prior summary and the reporter's reply, calls the drafter's clarification method, and creates exactly one Issue. It never produces further questions — the redraft always yields a `Ready` Draft.
- These are two use cases, not one method with a conditional. Their inputs, failure modes and results differ; they share only the Jira-write tail.

### Gemini adapter

- The response schema becomes an object containing `split_reasoning` (string) and `candidates` (array of objects), with `MinItems` 1 and `MaxItems` 5 on the array.
- `PropertyOrdering` is set so `split_reasoning` is generated **before** `candidates`. This is load-bearing twice over: it forces the model to commit to a segmentation before drafting, and `Schema.Properties` is a Go map, so without it field order is nondeterministic.
- Each candidate object carries: `status`, `app`, `title`, `description`, `issue_type`, `priority`, `open_questions`.
- The splitting rule, the vagueness bar and the labelling rule live in per-field `Description` values on the schema rather than in the prompt body, per the SDK's own guidance. They are structural and stable, which keeps the request prefix cacheable.
- The App Context is supplied to the adapter at construction and rendered into `SystemInstruction`, ahead of the conversation, so the request prefix stays stable across all requests.
- The model stays `gemini-3.6-flash`, per ADR-0001.

### The splitting rule

Split only where each piece could ship alone and someone would notice an
improvement. If a piece is only meaningful once another piece is done, it is a
step, not a Candidate — merge it. Feature area, drawn from the App Context,
serves as the tiebreaker: complaints landing in the same feature area are
usually one Candidate, complaints in different areas are usually two. The
default is one Candidate; a second requires positive evidence.

### The vagueness bar

Draft the Candidate if you can name something observable that should be
different. Missing repro steps, device, severity or exact screen are gaps to
note in the description — not reasons to ask. Ask only when you cannot tell
what the reporter wants changed.

### The labelling rule

`Backend` is chosen only on an explicit server, API or data-side signal from the
reporter, or when the symptom is app-agnostic — the same wrong data in both
apps, or a report with no UI surface at all. Otherwise the label records the app
the problem was observed in. The model never infers which layer a fix belongs
to.

### App Context payload

One static file in this repository covering the Mela app only: what the product
is, its feature list with a one-line description each, and the platform and
third-party services that appear in reports (Flutter, BLoC, Onfido, Stripe,
Plaid and similar). Roughly six hundred words. It is seeded from the Mela app's
own feature index but maintained here, because the other two products have no
equivalent index to sync from.

The hard boundary is **what exists, never how it works**. Feature names and
vendor names are in; internal architecture, file layout and routing logic are
out, because that is the material that tips the model from classifying into
diagnosing.

Merchant App and Backend get no context payload. Their Candidates are drafted
from the conversation alone, and quality is knowingly lower for them.

### Clarification carrier — no storage

There is no store. The bot's own question message is the state.

The question message contains a short marker, a one-line restatement of each
unresolved Candidate, and up to three questions. When the reporter replies to
it, the handler recovers everything it needs from the message it is replying to.

This is forced by two facts. Telegram caps reply nesting at exactly one level —
a reply to the bot's question yields the question, never the message the
question was about. And the Bot API has no `getMessage`, so a message ID alone
cannot be exchanged for content later. The carrier must therefore hold the
substance, not a pointer.

The trade is fidelity: the redraft works from the bot's summary rather than the
original thread. Acceptable, because the original was by definition too vague to
draft from, and the reporter can see and correct the summary in the same reply.

The marker is tamper-proof in practice: only a message's own sender can edit it
in Telegram, and the handler additionally requires that the replied-to message
came from the bot itself.

### Expiry without storage

A question expires 24 hours after it was asked. With no record to sweep, expiry
is evaluated from the replied-to message's own timestamp at the moment the
answer arrives. A late answer is ignored silently — no ticket, no error message.

### Telegram adapter

- The handler now inspects every message in an allowlisted chat, not only
  commands. A message is acted on when it replies to a bot message carrying the
  clarification marker; everything else is ignored without a Gemini call.
- Replies to `Created <KEY>: ...` confirmations continue to route to `/status`
  exactly as they do today. Marker detection mirrors the existing key-parsing
  helper — the same pattern, a second instance of it.
- One grouped reply per Ticket Request: created Issues with links, then
  failures, then the question. Not separate messages per outcome.
- Telegram caps a message at 4096 characters. When five Candidates would
  overflow it, per-Candidate summaries are truncated; Candidates are never
  dropped.

### Fan-out and failure

- `@assignee` is applied to every Issue from the message. The soft-fail to
  unassigned is unchanged.
- The source message's single image is attached to every created Issue.
- Failures are reported per Candidate. No rollback, and no duplicate protection:
  re-running the command on the same message creates a second set of Issues.
  The grouped reply is the only safeguard, which is why its clarity matters.
- Clarification-path Issues are created **unassigned and without the
  attachment**. Neither survives on the carrier, and both are cheap for a human
  to fix in Jira.

### Dependencies

`telegram-bot-api/v5` stays pinned at v5.5.1. Nothing in this design needs Bot
API 7.0 — `external_reply` and `quote` carry no message ID that helps, and the
one-level nesting cap is not something a newer version lifts.

## Testing Decisions

A good test here asserts what a caller observes: the Issues that got created,
the text the chat receives, the Draft parsed out of a response body. It does not
assert how many times a port was called, nor reach into unexported state. The
existing suite already works this way and is the prior art to follow.

Three seams, all of which already exist. No new seam is introduced.

**Use case, against fake ports** — prior art: `internal/usecase/create_ticket_test.go`.

- A DraftSet of one Candidate creates one Issue (the existing behaviour, unbroken).
- A DraftSet of three Ready Candidates creates three Issues.
- A DraftSet mixing Ready and NeedsClarification creates only the Ready ones and surfaces the question.
- A tracker failure on the second of three Candidates still creates the first and third, and reports the second as failed.
- The assignee is applied to every Issue; an unresolved handle still creates all of them.
- The attachment is passed for every Issue.
- The clarification use case produces exactly one Issue, unassigned, with no attachment.
- A drafter error is distinguishable from a tracker error via the existing sentinel errors.

**Telegram handler, against a fake bot client** — prior art: `internal/adapters/telegram/telegram_test.go`.

- An ordinary chat message triggers nothing.
- A reply to a bot confirmation still routes to `/status`.
- A reply to a marked clarification message routes to the clarification use case.
- A reply to a marked message older than 24 hours is ignored silently.
- A reply to a marked message that did not come from the bot is ignored.
- The grouped reply renders created links, failures and the question together.
- Five verbose Candidates render within Telegram's character cap without losing a Candidate.
- A message from a chat outside the allowlist is ignored — unchanged.

**Gemini response parsing, against fixtures** — prior art: `internal/adapters/gemini/gemini_test.go` and its `testdata/`.

- A multi-candidate response parses into a DraftSet in order.
- A single-candidate response parses.
- An unknown `app`, `status`, `issue_type` or `priority` is a parse error, matching how the existing enums already fail.
- A NeedsClarification candidate carrying no questions is a parse error.
- Malformed JSON is a parse error.

The splitting rule itself is a prompt behaviour, not a unit under test. It is
validated by scoring a sample of real Ticket Requests before and after — label
correct, area correct, description free of invented causes — not by asserting on
Gemini's output in CI.

## Out of Scope

- Any datastore. ADR-0001's no-database decision stands.
- Duplicate detection. Re-running the command creates a second set of Issues.
- Rollback or transactional creation across Issues.
- More than one round of clarification.
- Editing or confirming a Draft before creation. ADR-0001's fire-and-forget UX is preserved, not reopened.
- Context payloads for the Merchant App and the backend.
- Feature indexes in the Merchant App or backend repositories, and any cross-repo documentation sync.
- Assignee or attachment on the clarification path.
- Upgrading `telegram-bot-api` or moving to a maintained fork.
- Per-topic or multi-project Jira routing. Single project, per ADR-0001.
- Video handling, which remains link-only per ADR-0001.
- Any change to `/status`.

## Further Notes

**ADR work this spec implies.** ADR-0001 states "there is no confirm/edit step
before creation". The clarification loop needs that clause narrowed: fire-and-
forget governs everything the bot can draft, and a question is not a
confirmation because nothing sits pending approval. This should be recorded as
an amendment rather than left as a silent contradiction. Separately, the
stateless carrier — the bot's own message holding the state, expiry read from
its timestamp — is a decision with enough consequence to deserve its own ADR,
particularly the reasoning that Telegram's one-level nesting cap and the absent
`getMessage` are what rule out the alternatives.

**Glossary additions.** `CONTEXT.md` needs `Candidate`, `App Context`,
`Clarification` and `App` added, and its Draft entry updated: a Ticket Request
now yields a DraftSet rather than a Draft.

**The risk worth watching.** Over-splitting is the failure mode that will damage
trust fastest — four tickets from one rant, three closed as duplicates, and
nobody uses the bot again. Under-splitting costs a human ten seconds in Jira.
The one-Candidate default, the cap of five, and the forced `split_reasoning`
field are all guards against the same asymmetry, and if the rule needs tuning
after launch it should be tuned toward merging.

**The other risk.** Everything that makes drafts better also makes fabricated
diagnosis more likely. The App Context boundary — what exists, never how it
works — is the control. If invented root causes go up after launch, the fix is
to cut context, not add more.
