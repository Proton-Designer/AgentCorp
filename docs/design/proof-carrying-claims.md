# The Epistemic Auditor — detecting verification theatre in an agent mesh

> **Thesis (post-refutation).** In a mesh of cooperating LLM agents, the failures
> that matter are not wrong *claims* — they are **checks that could not have caught
> anything**: a test that passes regardless of correctness, a measurement equal to
> its own timeout, two "independent" sources that share a blind spot, a detector
> that has never once fired. The scarce primitive is not proof, it is **the cheap
> detection of fake verification.** This document specifies a passive **auditor**: a
> small set of domain-general detectors that, run over the checks a mesh already
> produces, flag the ones that cannot discriminate — without mandating any new
> per-message protocol. It asks *"is this CHECK trustworthy?"*, never the
> unanswerable *"is this CLAIM true?"*

## 0. Reading this honestly

This design was produced in a fast Opus↔Opus dialogue between two instances of one
model, then **deliberately attacked by a third agent given only the claim and
mechanism — none of the supporting story** — so its refutation could not be our
conclusion handed back. That attack reshaped the thesis (see §3). Every load-bearing
empirical claim carries a confidence tag (`VERIFIED`/`HIGH`/`MEDIUM`/`LOW`/`BARE`)
naming how it was checked. The only genuinely independent inputs were a literature
scan and the cold refutation; both are weighted above internal agreement. A specimen
of the exact failure this auditor targets — produced *by the design process itself,
between two agents actively watching for it* — is recorded in §2.3 and is the single
strongest piece of evidence in this document.

## 1. The problem: fake verification, not wrong claims

Every message an agent sends is an assertion a receiver can only check by redoing the
work. The industry's response (A2A, ACP, ANP, ExtAgents, context compression) scales
the *throughput of assertions*. `MEDIUM` (literature scan.) None of it addresses that
the assertions are unfalsifiable — and, more sharply, that **the verifications agents
run to support their claims are frequently themselves broken.** The dangerous failures
are honest, and they live in the verification, not the claim:

- A measurement equal to its own loop-exhaustion budget (`200 × 100ms = 20.5s`),
  honestly reported — the instrument returned its own cap as data.
- A CI gate "protecting" a canary that was never observed failing — it could not have
  detected the canary being deleted.
- A regression test that **passed with its own regression reintroduced** — it raced an
  async effect and checked before the effect could occur, so it passed regardless of
  correctness.
- Two renderers "agreeing" a glyph is one cell wide — because *neither implements* the
  feature that would make it two.
- A path check run against the wrong resolved path — green, and meaningless.

Crucially, the **threat model is honest error, not malice.** The adversarial agentic
web is well served by cryptographic integrity (signatures, zk-proofs, DIDs, verifiable
credentials). `HIGH` A signature over the 20.5s number is *valid*; the number is still
garbage. **Integrity attestation is orthogonal to correctness.** None of these
failures is a lie; each is a check that looked like it verified something and did not.

## 2. The insight, and what survived refuting it

### 2.1 The first idea (refuted)
The dialogue first reached: *make every inter-agent claim carry a cheap falsifier — a
necessary condition that must hold if it's true — and score "falsifier-coverage."*
This is [metamorphic testing](https://dl.acm.org/doi/10.1145/3143561) (Chen et al.,
1998 `VERIFIED`) applied to coordination, with [proof-carrying code](https://en.wikipedia.org/wiki/Proof-carrying_code)'s
economics (shift the burden to the producer; verification is cheaper than generation).

### 2.2 Why it was killed
A cold refutation (independent agent, no story) landed the load-bearing flaw:
**falsifier-coverage measures claim TYPE, not correctness.** Mechanical/narrow claims
are easy to falsify *and* already usually true (that's why they're mechanical); the
holistic judgments most likely to be expensively wrong resist cheap falsifiers or wear
**fig-leaf** ones ("the fix works" → "the file was modified": necessary, cheap,
useless). So coverage tracks surface-form narrowness, not truth — a team hits 95% by
tagging trivia while the one architectural claim that breaks things sits unflagged.
Textbook Goodhart, and it *inversely* targets the claims that matter. Mandating
falsifiers builds a theatre generator. `HIGH` (the argument is sound; see §5.)

### 2.3 What survived — and a live specimen
The kernel the refutation *left standing* is better than the thesis it killed: don't
mandate proofs on claims — **detect broken verification.** The right question is not
"is the claim true" (uncheckably expensive) but "**could this check have caught
anything**" (cheap, mechanical, and exactly the shape of every real bug this project
has caught). Nobody re-verified the claims above; someone noticed the *verification*
was theatre.

The strongest evidence this failure is real and hard to see was produced *by writing
this document*: mid-design, the author claimed "both citations, with the numbers you
gave" — but the collaborator gave **no numbers**; they came from a web search. Two
"sources" (a recollection and a search) sharing one underlying dependency were read as
independent corroboration — the exact correlated-provenance failure of §4, committed
in the sentence claiming to guard against it, between two agents actively watching for
it. If a failure can happen *there*, "just be careful" is not a control.

## 3. The auditor: four detectors of non-discrimination

Each detector is O(1)–O(cheap), domain-general, and answers "this check could not have
discriminated." Each records **which** signal fired (a bare "flagged" is itself
unfalsifiable one level up).

1. **Boundary-equal** *(strongest — keep even if you keep only one)*. A measurement/
   result exactly equal to a known timeout, cap, budget, or loop bound. Catches a
   clamped value masquerading as data (`20.5s == 200×100ms`).
2. **Non-discriminating check** *(the fig-leaf killer)*. A check that has never
   returned its interesting (failing) value, generalized to: a check that **does not
   fail when the thing it verifies is broken.** This is a cheap mutation probe — seed
   a known break, re-run the check; if it still passes, it is a fig leaf. Catches the
   sabotage test that couldn't fail and the async-race test that always passed.
3. **Cost-surprise**. A result cheaper/faster than predicted (a test that finished
   before the effect it checks could occur). Requires a logged prediction; noisy — the
   weakest of the four, and flagged as such. `LOW`
4. **Correlated provenance**. Two agreeing checks whose dependency sets intersect;
   agreement is evidence only if derivation paths are disjoint. **Open limitation
   (from the refutation):** in an LLM mesh the dangerous correlation is often shared
   *training data or prompt-induced bias*, not a nameable dependency — this detector
   catches the software-supply-chain shape, not that one. We do not have a cheap
   detector for prompt-bias correlation; stated plainly rather than papered over.
5. **Missing boundary coverage** *(the absence detector — build first)*. Detectors 1–4
   are all negative signals over checks that *exist*; none can see a control that was
   never written, and absence-of-check is the more dangerous half. This one is a graph
   property, not a per-check test: does any test path reach a capability *through the
   real transport* (not a mock)? A capability implemented, unit-tested in-process, and
   **unreachable across its layer boundary** passes every other detector because there
   is nothing to flag. Reachability topology, statically computable, and it is exactly
   the class of the most expensive honest-error failures.

## 4. Invariants (protect under pressure)

- **The auditor never asserts a claim is true.** A check surviving all four detectors
  is *not proven trustworthy* — the detectors are necessary, not sufficient (the
  metamorphic-testing bound). It has merely not been caught. No terminal "verified"
  state exists. This is the first thing that will feel like friction and the thing to
  defend hardest.
- **Record the signal, not just the verdict.**
- **Passive, not mandatory.** The auditor runs over checks that already exist. It adds
  no message-schema every agent must carry — precisely the mandate the refutation
  killed. Agents *may* attach a falsifier when a claim naturally has a cheap shadow;
  it is an affordance, never a scored quota.
- **No reputation leaderboard.** Scoring per-agent "standing" recreates Goodhart
  (hedge, avoid checkable claims, pad with trivia). The auditor flags *checks*, not
  agents. If any agent-level signal is surfaced it is retraction-positive only:
  self-correction must never cost more than being quietly wrong.
- **Diagnostic, not a KPI.** Every detector has a cheap defeat the instant it is
  *scored* — fire the never-fired detector once on a trivial case, pick a cap you'll
  never hit. So detections are surfaced as **findings to investigate**, never as a
  number an agent optimizes. In particular a "theatre count" must NOT sit on the
  vitals HUD next to the verification rate: the moment it's a company metric, it's a
  target, and the detectors become the next thing gamed. A finding names a specific
  check to look at ("this test did not fail on a seeded break"); it is not a grade.

## 5. Validation — the auditor's own falsifier

The thesis must carry one, and the naive experiment is undefined (a claim with no
check has nothing to audit) and confounded (checkability selects for claim-type). So:
**retrodiction against a labelled failure set.** Take real, ground-truth honest-error
failures — this project's own (the inferred-never-observed width phantom; the
stale-binary claim; the byte-vs-rune "asymmetry" that made correct centering look
broken; the demo-hire false lead) plus the verification-theatre cases above — and for
each ask: *would one of the four detectors have fired, mechanically, with no review?*
Count. Strong support if most; a narrow result kills the auditor cheaply, which is the
property we want. **Confound control:** labelling is done from the record alone by an
agent that did not live the failures (hindsight makes falsifiers too easy to fit). It
is retrodiction — weaker than prediction — stated plainly.

### 5.1 Pre-registration (written before the harness ran)

Recorded here so the result can disconfirm it. Two confound controls: retrodiction
records are encoded **from artifacts** (real test files, commits, repo state), not
from the postmortem narrative — a record that can't be built from what existed at the
time is itself a finding; and the **prospective** run (detectors 2 and 5 over the live
repo) is prediction, not retrodiction, and weighted above it.

Predictions:
- **Retrodiction hit rate: ~9–10 of 14.** Predicted MISSES (~4): the byte-vs-rune
  miscount (wrong *unit*, no detector targets it), the path-check against the wrong
  resolved path (wrong *target*, same gap), and one or both bare-assertion phantoms
  (no check existed to audit).
- Detector #1 boundary-equal: fires on the cap-equal measurement. #2 non-discriminating:
  the strongest, fires on the canary gate, the async-race test, the never-true overflow
  detector, the colour-blind md5 capture. #4 correlated-provenance: the shared-missing-
  feature agreement. #5 boundary-coverage: the unreachable-over-the-wire cases.
- **Prospective (the real bet): #2 finds ≥1 genuinely non-discriminating test in
  AgentCorp's own suite that I did not know about, and #5 finds ≥1 exported capability
  with no test reaching it through the real path.** If both find zero on a suite this
  size, the auditor mostly finds nothing and that is the disconfirming result.

### 5.2 The pitch

The honest pitch, sized to what's defensible: *the technique is old (metamorphic
testing formalized the shadow idea in 1998; Knight & Leveson established correlated
N-version failure in 1986 `MEDIUM`, one source; a 2026 paper reportedly reproduces it
in coding agents yet finds 3-version voting still cuts failures ~66% `MEDIUM`, primary
source unread). What is new is a passive, domain-general theatre-detector applied to a
local cooperative agent mesh, validated by retrodiction against real failures, and
surfaced as the epistemic observability of an agent company.*

## 6. AgentCorp integration — the audit function of a company

A real company's auditor does not re-do every transaction; it **tests whether the
controls are real.** That is exactly this. The auditor becomes AgentCorp's epistemic
observability:
- A runnable `audit` over the project's own checks (dogfooding — pointing the
  theatre-detector at ourselves; a fig leaf found in our own suite is the proof).
- Claims/checks flowing in the mesh render with an audit state: **clean** vs
  **flagged (non-discriminating / boundary-equal / correlated)** — a distinction no
  current system surfaces.
- The flags are a company vital: how much of what this company "verified" could not
  have caught anything.

## 6.5 Results, and what is honestly novel (and what is not)

Built and run, on two codebases, blind-graded across teams. The results are real and
small, and the framing has been corrected once already — recorded here because the
correction is the more useful artifact.

**What the auditor found:**
- *Retrodiction* (§5): 9/14 real honest-error failures caught by ≥1 detector,
  pre-registered; weak (encoding confound).
- *Prospective, AgentCorp:* boundary-coverage flagged 7 zero-coverage capabilities
  (cold-graded 4 strong / 2 probable / 1 uncertain), including a documented invariant
  with no test (`store.SetState` — guard confirmed holding by a test written in
  response), the disband cascade-kill (dialog tested five ways, the kill never
  invoked), and broadcast (targeting tested, sending not). The discrimination probe
  flagged 2 surviving mutants — `matchesFilter`'s never-exercised empty-filter branch
  and `BuildTree`'s untested tie-break — and confirmed clean discrimination on
  `internal/vitals` and `internal/hire`.
- *Cross-codebase (the strongest test — foreign code, blind grading):* the
  invariant-comment detector, run on a peer team's TypeScript codebase the author had
  never opened, graded cold by that team: **2 of 19 genuine** (a *fixed-but-never-
  tested* bug in `colorKey`; an untested fatal-propagation in `resolveWidthTable`),
  ~15 false positives, precision ≈ 11%.

**What is NOT novel (stated so the write-up can't overclaim it).** The pattern the
findings trace — tests cover decisions and main paths, skip effects and edge branches
— is not a discovery about *agent* teams. It is the founding premise of **mutation
testing** (DeMillo, Lipton & Sayward, 1978) and the reason branch coverage and MC/DC
exist. On this evidence we cannot distinguish "a property of agent-written code" from
"a property of code"; a human team's suite would likely show the same shape. So the
claim is *not* "agent teams have this bias." And with four findings read off the
results after seeing them, the pattern is **consistent, not replicated** — replication
would need a pre-registered prediction that could have failed (e.g. "the next run on a
third codebase yields >60% edge/effect cases," then run). We did not make that
prediction; we will not pretend we did.

**What IS novel, narrowed to what survives.** Not the detection — coverage analysis
and mutation testing have decades on this. What has no clear prior art is the **loop**:
an auditor whose findings are routed *between independent agents as a coordination
signal* — one agent's tool auditing another agent team's codebase, a third party
grading the results **blind**, with **pre-registered predictions on both sides** that
were allowed to fail (and did: the grader's headline prediction about the failure
mechanism was wrong, and the tool's author's corroboration claim was manufactured and
retracted). Cross-agent, blind-graded, pre-registered verification as a working
protocol is the contribution. The detectors are its cheap, old, honest instruments.

**Honest limitation (from the cold grade).** The ~11% precision comes not from the
language grep (contract-vs-rationale precision was good) but from the **coverage
proxy**: symbol-reference asks "does a test NAME this," but well-factored code tests
internals THROUGH their public entry point, so **precision falls as factoring
improves** — a perverse gradient penalizing the structure you want. The fix (not built
here) is call-graph reachability, the same graph property as boundary-coverage one
level down. At this precision the detector is a diagnostic that surfaces "N to
investigate," never a gate or score — mandated, it buries the real hits and the first
user stops reading.

**The standing caveat, unresolved.** This design and its validation were produced by
two instances of one model converging over hours. The single genuinely independent
input was a literature scan, and it corrected the authors twice. Every substantive
improvement tonight came from one party being wrong *on the record* and another
noticing — which is simultaneously the strongest evidence for the auditor's premise
and the reason to distrust the authors' agreement about it.

## 7. What we build (scope)
1. `internal/audit` — the four detectors as pure functions over a general `Check`
   abstraction (claim, result, cap, duration, predicted, deps, discriminates), each
   returning a `Finding` naming the signal. Tested.
2. A **discrimination prober** — cheap mutation over a target check: seed a break,
   re-run, flag if it still passes. The fig-leaf killer, runnable on real tests.
3. The **retrodiction harness** — real labelled failures encoded as `Check`s; assert
   the detectors fire. The thesis eating its own dog food.
4. AgentCorp surface — a runnable audit + the flagged-vs-clean observability.

All honesty-first, confidence-tagged, no terminal "verified" state, no leaderboard.
