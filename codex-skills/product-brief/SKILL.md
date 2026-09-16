---
name: "product-brief"
description: "PRODUCT child (invoked via $product): the stage-0 brief — a working-backwards PRFAQ that states the idea as a shipped thing, then returns a go / needs-clarification / kill verdict before any further stage runs."
---


# $product-brief — stage 0: is this worth the other seven stages?

**Version:** product family 1.0.0

<!-- codex-port: Codex frontmatter permits only name and description, so the
     version lives here in the body. Read it from this line when stamping a
     plan's planner-version / executor-version. -->


A PRFAQ written as though the product already shipped. Amazon's rule is the whole point: **if you cannot explain why this matters to a customer before it exists, you should not build it.** This stage is cheap and the seven after it are not, so it is allowed to say no.

## Dependencies

Invoked with Codex's explicit `$name` syntax. Each must also exist under Codex's skill-discovery path or the call will not resolve:

- `$product` — ported.

## Steps

1. **Gather the idea.** In brownfield mode, read the code and README first and state what the product already appears to be — the user corrects rather than dictates. In greenfield mode, ask for: the customer, their problem today, what they do instead right now, and what changes for them if this exists. Nothing else yet.
2. **Write the press release as though it shipped** — dated, past tense, no hedging, no feature list. If it reads as exciting only to the people building it, that is a finding, not a style problem.
3. **Write the FAQs.** Customer FAQs are what a buyer asks. Internal FAQs are the hard ones — and they are where the verdict comes from. Every internal FAQ you cannot answer becomes a `[NEEDS CLARIFICATION]`.
4. **Reach a verdict** and state it plainly with its reason.

## Output — `docs/brief.md`

```markdown
# <Product> — Brief

**Date:** <YYYY-MM-DD>  ·  **Verdict:** go | needs-clarification | kill

## Press release
### <Headline: the customer benefit, not the product name>
**<City>, <date>** — <opening paragraph: what launched and who it is for.>

<Problem paragraph: the customer's world today, concretely.>

<Solution paragraph: what they do now instead, in plain words.>

<Customer quote — the outcome in their voice, not marketing copy.>

<How to get started.>

## Customer FAQ
**<What a buyer actually asks>**
<Answer.>

## Internal FAQ
**What has to be true for this to work?**
**What is the hardest part, honestly?**
**Who is already doing this, and why haven't they won?**
**What would make us kill this in six months?**
**What does success look like in numbers?**
**What are we deliberately not doing?**

## Verdict
**<go | needs-clarification | kill>** — <the reason, in two or three sentences.>

## Open questions
- [NEEDS CLARIFICATION] <each internal FAQ you could not answer>
```

## The verdict

| Verdict | When | What happens |
|---|---|---|
| `go` | The press release lands, the internal FAQs are answered, the customer and their problem are specific. | The chain continues. |
| `needs-clarification` | The idea is plausible but load-bearing facts are missing — no identified customer, no articulated problem, unanswerable hard questions. | **Stop.** Name what is missing. This stops `--fast` too, because everything downstream would inherit the guesswork. |
| `kill` | The problem isn't real, isn't the customer's, is already solved well, or the honest answer to "what's the hardest part" is "the part we have no way to do". | **Stop and say so.** Record the reason — a killed idea documented is worth more than a killed idea forgotten. |

## Rules

1. **A verdict is required.** "Looks interesting" is not one.
2. **`kill` is a real outcome, not a failure of nerve.** It is the cheapest thing this family can produce.
3. **No feature lists.** A press release that lists features has skipped to stage 3.
4. **No technology.** Not here, not in the FAQs.
5. **Write the quote as a customer would say it** — if no plausible customer would say it, the benefit isn't one.
6. **Never soften an internal FAQ** to make the verdict easier. The hard questions are the deliverable.
