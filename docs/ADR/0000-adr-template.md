# ADR-0000: Template for Architecture Decision Records

<!--
Template based on Michael Nygard ("Documenting Architecture Decisions", 2011),
extended with sections from MADR 4 (https://adr.github.io/madr/).

How to use it:
- Copy the file as `NNNN-short-title-in-kebab-case.md`. The number is
  sequential, four digits, and never reused.
- Exactly one decision per ADR. Phrase the title as a statement
  ("We use X for Y"), not as a question.
- An ADR with status "Accepted" is no longer changed in substance. If the
  decision changes, write a new ADR. The old one then gets the status
  "Superseded by ADR-NNNN" and a link to it. Typos and links may be fixed
  at any time.
- Keep it short: an ADR records why a decision was made. Documentation of
  the how belongs elsewhere.
- Delete optional sections if they add nothing.
-->

- **Status:** Proposed | Accepted | Rejected | Deprecated | Superseded by [ADR-NNNN](NNNN-title.md)
- **Date:** YYYY-MM-DD (last status change)
- **Deciders:** Who decided?
- **Involved (optional):** Who was consulted or informed?

## Context and Problem Statement

What situation forces the decision? Which technical, organizational and
domain constraints apply? Describe it neutrally here, without judging the
options yet. End with the question the ADR answers.

## Decision Drivers

- Driver 1, e.g. a quality requirement or a hard constraint
- Driver 2
- …

## Considered Options

1. Option A
2. Option B
3. Option C

## Decision

We chose **Option X** because … (rationale referring to the decision drivers).

Phrase the decision in the active voice: "We use …", "We do without …".

## Consequences

### Positive

- …

### Negative

- …

### Neutral / Follow-ups

- …

## Confirmation (optional)

How do we know the decision has been implemented and holds up?
Examples: review, test, fitness function, metric.

## Pros and Cons of the Options (optional)

### Option A

Short description.

- Good, because …
- Neutral, because …
- Bad, because …

### Option B

…

## More Information (optional)

Links to related ADRs, issues and sources. Open questions and when the
decision should be revisited.
