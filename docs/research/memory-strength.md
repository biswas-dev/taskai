# Memory strength and belief: research, design and eval

Status: research prototype behind `MEMORY_STRENGTH` (off by default). The protocol version stays `1`. Date: 2026-11-23.

## Summary

The question: is a flat memory store useful, or must each memory get a strength score (from use) and a confidence score (from evidence)?

The recommendation, in five lines:

1. Keep the flat store and its hybrid search. It finds the right memory (recall@10 0.94 in our synthetic world). The errors are in the order of the results, and most are **stale** memories.
2. Fix staleness first. A store that shows the writer **similar older memories on write**, so the writer can supersede them, raised hit@1 from 0.748 to 0.772-0.794 with no feedback at all. The result depends on how often the writer acts (50% to 90%; threshold 0.80; one grid, run on the test seeds). An oracle that supersedes every update reaches 0.838.
3. Strength plus belief, as a bounded re-rank, add about 4.5 hit@1 points with good feedback (better on 5 of 5 seeds), about 2 points with noisy feedback, and about 2 more on top of similar-on-write. They help most when the same facts are asked again and again. Without feedback, strength is recency. On LongMemEval, recency puts the newest of two answer sessions first in 58 of 72 knowledge-update questions (flat: 38 of 72). The cost is 2 of 222 questions at rank 1 overall, and 24 of 1,026 known-item queries in a real store.
4. The belief score is **not calibrated** without labels (ECE 0.114). Platt scaling on labelled feedback brings it to 0.035, but only when the feedback behaves as in training (0.092 under a shift).
5. Ship similar-on-write and the feedback counters first: they are cheap and they collect the labels. Turn the re-rank on only after real feedback confirms a gain.

| Variant (synthetic world, held-out seeds) | hit@1 | MRR | stale top-1 |
|---|---|---|---|
| Flat (current search) | 0.748 | 0.826 | 10.7% |
| + strength and belief re-rank | 0.793 | 0.851 | 8.0% |
| + similar on write (writer acts 50% / 90%) | 0.772 / 0.794 | — | 8.0% / 5.6% |
| + both of the above (writer acts 90%) | 0.816 | — | 4.3% |
| Oracle supersede (upper bound) | 0.838 | 0.880 | 1.0% |

The server ranking equals the eval's ranking on 2,922 of 2,922 queries, and adds no measurable latency (p50 49 ms).

The sections below give the research with citations, the design, the experiments, the risks and what we did not do.

## 1. What the research says

The full literature report, with a primary source for each claim, is [memory-strength-literature.md](memory-strength-literature.md). The short version follows.

**No agent-memory system has measured the gain of a per-memory score.** Generative Agents scores recency (0.995 decay per game hour), LLM importance and relevance with equal weights. MemoryBank uses an Ebbinghaus strength. MemoryOS uses a "heat". All three ablate whole modules, never the score alone ([Park et al. 2023](https://arxiv.org/html/2304.03442v2), [MemoryBank](https://arxiv.org/html/2305.10250), [MemoryOS](https://arxiv.org/html/2506.06326)). Mem0, Zep/Graphiti, A-MEM, HippoRAG and Memory-R1 keep no strength or confidence field ([Mem0](https://arxiv.org/html/2504.19413), [Zep](https://arxiv.org/html/2501.13956), [A-MEM](https://arxiv.org/html/2502.12110)).

**Flat and verbatim stores do well.** In Mem0's own LoCoMo harness, full context (J 72.90) beat every memory system, and the flat Mem0 (66.88) came within 1.6 points of its graph variant ([Mem0](https://arxiv.org/html/2504.19413)). Agents that keep raw episodes doubled the accuracy of agents forced to consolidate ([Zhang et al. 2026](https://arxiv.org/abs/2605.12978)). This supports our rule that compaction never merges similar text.

**The measured failures are stale and conflicting facts.** Systems omit most updates (52-98% omission on HaluMem) and rarely invent them (below 1.2%) ([HaluMem](https://arxiv.org/html/2511.03506v2)). Reader models follow stale notes (reliance 0.92-1.00 on the paper's benefit suite), and resolving the conflict before the model reads gives near-ceiling accuracy ([Memory Trust Gap](https://arxiv.org/abs/2609.01852)). A frozen last-write-wins rule answers 80.25% of MemoryAgentBench FactConsolidation questions (74.5% on held-out lists) ([arXiv 2610.09193](https://arxiv.org/pdf/2610.09193)). Zep invalidates old edges with validity intervals ([Zep](https://arxiv.org/html/2501.13956)).

**ACT-R strength has evidence, but outside agent memory.** Base-level activation B = ln(Σ t_j^-d), d = 0.5, follows the statistics of need in the environment ([Anderson & Schooler 1991](https://dyurovsky.github.io/learning-humans-machines/class/13-class/papers/anderson1991.pdf)). On hashtag reuse it beat recency-only by about 3-5% and frequency-only by about 9-42% ([Kowald et al. 2017](https://ar5iv.arxiv.org/html/1701.01276)). Spaced uses count more than massed uses ([Pavlik & Anderson 2005](https://kilthub.cmu.edu/articles/journal_contribution/Practice_and_Forgetting_Effects_on_Vocabulary_Memory_An_Activation-Based_Model_of_the_Spacing_Effect/6617648)). FSRS and Half-Life Regression need a graded outcome per review, which a memory store does not have ([srs-benchmark](https://github.com/open-spaced-repetition/srs-benchmark), [Settles & Meeder 2016](https://aclanthology.org/P16-1174.pdf)).

**Calibrated confidence needs labels.** The smallest model is a Beta-Bernoulli counter, E[p] = (r + aW)/(r + s + W), as in subjective logic ([Jøsang](https://en.wikipedia.org/wiki/Subjective_logic)). Calibration needs labelled outcomes: Platt scaling works with a few hundred labels, isotonic regression needs about 1,000 ([Niculescu-Mizil & Caruana 2005](https://www.cs.cornell.edu/~alexn/papers/calibration.icml05.crc.rev3.pdf)). LLM-stated confidence clusters at 80-100% and separates right from wrong only weakly (GPT-4 AUROC 62.7%) ([Xiong et al. 2024](https://arxiv.org/pdf/2306.13063)). Our own store agrees: about 95% of real memories say `high`.

**Contradiction detection is cheap only as a candidate step.** A generic NLI cross-encoder scores about 90% on NLI tests but 77.4% on dialogue contradictions ([DECODE](https://arxiv.org/pdf/2012.13391)). LLM judges are precise but miss many contradictions ([arXiv 2504.00180](https://arxiv.org/html/2504.00180v1)). Readers adopt wrong retrieved content more than 60% of the time ([ClashEval](https://arxiv.org/abs/2404.10198)), so the store must link and show conflicts, not merge them.

**Feedback loops are a real risk.** A poisoned memory below 0.1% of the store reached 63% end-to-end attack success ([AgentPoison](https://arxiv.org/html/2407.12784v1)). If being retrieved counts as a confirmation, a wrong memory gets stronger each time it ranks high.

**How we differ from MemPalace.** [MemPalace](https://github.com/mempalace/mempalace) stores verbatim text in a method-of-loci structure (wings, rooms, drawers) and reports retrieval recall on sets it tuned against. It keeps no strength or confidence. We keep the flat, per-user store with scope filters, add no spatial structure, and add two measured signals and a write-time validity aid. We share its "keep the raw text" rule.

## 2. Design (additive to protocol 1)

All parts are behind `MEMORY_STRENGTH`. With the flag off (the default), the API and MCP behave as before.

| Part | What it does | Why |
|---|---|---|
| Strength | For `fact`, `decision` and `handoff` (owner decision, 2026-11-24, after the recency results in 3.4 and 3.5): ACT-R base-level activation with Petrov's (2006) hybrid approximation at k = 1: the last use is exact, and the older uses are assumed evenly spread between the write and the last use. d = 0.5, time in days, minimum age 1 hour. The write counts as the first use. | Evidence above. Three columns (`use_count`, `last_used_at`, `created_at`) and no use log are enough. |
| Steady strength | For `gotcha` and `runbook`: ln(uses) + the strength of a new memory. Age does not lower it. | These lessons stay true until superseded; age-based strength cost 24 of 1,026 known-item searches in the real store. |
| Belief | Beta-Bernoulli posterior (confirmed + a·2)/(confirmed + wrong + 2), with a = 0.6 / 0.7 / 0.8 for stated low / medium / high. | The smallest evidence model. Named `belief`, not `confidence`, because it is not calibrated. |
| Feedback | `POST /api/memories/{id}/feedback {signal, updated_at?}` with signal `useful`, `confirmed` or `wrong`; MCP `memory_feedback`. Owner only; active, unexpired memories only. An optional `updated_at` (from `get_memory` or the REST memory) makes a changed memory answer 409. | The only source of signal. Search results never count as use. |
| Staged flag | `MEMORY_STRENGTH=off` (code default), `collect` (feedback, signals, similar-on-write; flat search; the `docker-compose.yml` default) or `on` (also the re-rank). | Lets the store collect labels before the re-rank changes any result. |
| Vote rules | `confirmed` and `wrong`: one vote per memory, signal and credential. `useful`: at most once an hour per memory. New text resets the votes and keeps the uses. A replay keeps everything. | Stops loops and retries from stacking votes. Votes judged the old text. |
| Re-rank | Search fetches a pool of at least 50 relevance candidates and adds strength and belief as two extra RRF legs with weights 0.5 and 0.25. With a limit of 2 or more, the most relevant candidate always stays in the results; a single slot goes to the best combined score. | Bounded: a signal can reorder relevant memories, never add an irrelevant one or hide the best match. |
| Similar on write | A write that creates or changes a memory returns up to 3 active memories of the same repo with cosine ≥ 0.80. | The cheapest measured fix for stale results. Nothing is merged automatically. |
| Compaction | A folded copy's feedback rows move to the kept copy (still one vote per credential) and its counters are recounted. A memory with any vote (confirmed or wrong), or used in the last 90 days, is not merged into a low-confidence summary. | Compaction must not lose evidence or bury memories in use. |

Storage: four integer/timestamp columns (about 24 bytes per memory) and one `memory_feedback` row per signal.

**What we skip, and why.**

- An NLI model or an LLM judge for contradictions: no model in the stack, 77% transfer accuracy on dialogue, and about 0.7 s per request on 16 CPU threads for a large model (search summary only). Similar-on-write with the writer's judgment does the job now.
- A stored "contradicts" link: the request asked for "detect and link". We detect candidates and return them on write, but we do not store a link. Embeddings cannot tell a conflict from a restatement (at most 37% of shown pairs are conflicts), so a stored link would mostly be wrong. The writer turns a real conflict into a supersede link.
- Age, kind and source in the belief prior: the prior uses only the stated level. We found no data to fit per-kind or per-source priors, and age is already in strength.
- Source reliability (truth discovery): one owner's credentials are not independent sources. Revisit with sharing.
- Bi-temporal `valid_from` / `invalid_at`: supersede already removes replaced memories from search. Add valid time when clients send it.
- Automatic votes from citations: no client reports citations yet. Being returned by search is deliberately not a signal.
- Strength-based deletion or decay of storage: no evidence supports it.
- A displayed calibrated probability: not possible until labels exist.

## 3. Experiments

### Harness

`api/internal/memrank/memeval` (not shipped) drives a **local** TaskAI API on Postgres + pgvector with the production embedding model (`all-minilm:l6-v2` through Ollama). The flat baseline is the shipped search, not a copy. The server and the eval share one scoring package (`api/internal/memrank`). The harness refuses a production URL and stops if search falls back from hybrid mode.

Three suites:

- **synth**: a generated world (public, no private data). 40 services × 8 settings; values change over 120 days; four writer agents with error rates 3%, 8%, 20% and 40%; writers supersede the old value only 40% of the time; restatements and 300 distractor notes. 1,500 queries per seed ask for a setting's current value. A simulated agent gives feedback on the top result. Weights are chosen on seeds 1-3 and reported on seeds 4-8.
- **lme**: LongMemEval_S (MIT), sessions as memories at their dates, scored at session level against `answer_session_ids`. No feedback exists here, so strength is creation recency only.
- **owner**: one real store of about 1,000 memories, loaded only into the local API. It prints aggregates only. With no use log and no correctness labels it can check only "no harm", latency, storage and conflict-candidate rates.

### 3.1 Synthetic world: does the re-rank help?

Held-out seeds 4-8, 7,500 queries, one feedback credential (as the server counts it):

| variant | hit@1 | recall@5 | recall@10 | MRR | hit@1 repeat | hit@1 first ask | top-1 stale | top-1 wrong |
|---|---|---|---|---|---|---|---|---|
| flat | 0.748 | 0.926 | 0.943 | 0.826 | 0.743 | 0.781 | 0.107 | 0.087 |
| recency only (no feedback) | 0.762 | 0.925 | 0.941 | 0.831 | 0.761 | 0.770 | 0.052 | 0.077 |
| strength w=0.1 | 0.761 | 0.927 | 0.943 | 0.833 | 0.757 | 0.787 | 0.098 | 0.083 |
| belief w=0.25 | 0.769 | 0.927 | 0.942 | 0.837 | 0.768 | 0.778 | 0.097 | 0.074 |
| strength 0.5 + belief 0.25 | **0.793** | 0.928 | 0.942 | 0.851 | 0.794 | 0.786 | 0.080 | 0.066 |
| oracle supersede | 0.838 | 0.933 | 0.940 | 0.880 | 0.839 | 0.832 | 0.010 | 0.081 |
| oracle supersede + both | 0.850 | 0.934 | 0.940 | 0.886 | 0.854 | 0.820 | 0.009 | 0.069 |

Paired change in hit@1 against flat, per test seed:

| variant | mean | min | max | seeds better |
|---|---|---|---|---|
| recency only | +0.014 | -0.007 | +0.031 | 3/5 |
| strength w=0.1 | +0.013 | +0.005 | +0.021 | 5/5 |
| belief w=0.25 | +0.021 | +0.011 | +0.034 | 5/5 |
| both | +0.045 | +0.031 | +0.065 | 5/5 |
| oracle supersede | +0.090 | +0.062 | +0.119 | 5/5 |
| oracle supersede + both | +0.102 | +0.077 | +0.127 | 5/5 |

**How the weights were chosen.** A grid of 0.1, 0.25, 0.5 and 1 for each weight, scored on train seeds 1-3. The best pair on train was strength 0.5 + belief 0.5 (hit@1 0.780); strength 0.5 + belief 0.25 scored 0.776, inside the seed noise. We ship belief 0.25. Be aware that we made this choice after we saw the noisy-feedback results, where belief 0.5 keeps no gain (held-out 0.789 with good feedback, 0.751 with noisy feedback, against 0.793 and 0.766 for belief 0.25). Both are reported. Weights of 1 or more hurt (strength 1 + belief 0.25: 0.746 on train).

What this shows:

- Recall@5 and recall@10 do not move: the signals reorder, they do not find more. The gain is all in the first position.
- On the first ask of a setting (no feedback yet), the re-rank is neutral (0.786 against 0.781). It does no harm there.
- The re-rank is bounded by design: a signal leg adds at most weight/61 to a relevance score, so it reorders near-ties only. Two copies of one text that are one rank apart in two relevance legs do not swap.
- Recency alone halves stale answers but promotes new distractors (top-1 "other" 5.9% → 11.0%), and is not reliably better (3 of 5 seeds).
- Effects below about 2 points are within seed noise; the 4.5-point gain of both signals is not.

**Noisy feedback.** A mis-specified agent confirms less, catches fewer errors and cites more wrong memories:

| variant | hit@1 (same agent) | hit@1 (noisy agent) |
|---|---|---|
| flat | 0.748 | 0.748 |
| belief w=0.25 | 0.769 | 0.751 |
| both | 0.793 | 0.766 |

The gain depends on feedback quality. Belief alone almost vanishes under noise.

**Credentials.** One vote per credential barely matters here: both signals score 0.793 with 1 credential, 0.789 with 3, 0.785 with no cap.

**Need distribution.** The share of queries that repeat a recently asked setting (rho) decides the value of strength:

| variant | rho 0 | 0.25 | 0.5 | 0.75 | 0.9 |
|---|---|---|---|---|---|
| flat | 0.744 | 0.746 | 0.748 | 0.779 | 0.794 |
| belief w=0.25 | 0.767 | 0.773 | 0.769 | 0.795 | 0.813 |
| both | 0.767 | 0.779 | 0.793 | 0.817 | 0.851 |
| oracle supersede | 0.835 | 0.835 | 0.838 | 0.860 | 0.883 |
| oracle supersede + both | 0.832 | 0.839 | 0.850 | 0.869 | 0.898 |

When queries do not repeat (rho 0), the re-rank adds about 2 points; when they repeat a lot (0.9), about 6. Agents that work on the same repository for days are near the right side.

Caution: the simulator decides how feedback relates to truth, so it cannot prove the gain in real use. It shows the size of the gain under stated assumptions and where it disappears.

### 3.2 Similar memories on write

At each write, the store shows the writer the 3 nearest earlier memories of the same scope, if they are above a cosine threshold (as the API does). The simulated writer supersedes a shown memory that states another value of the same setting with probability *act*. A wrong write can retire a right memory; that cost is in the numbers.

| threshold | act | flat hit@1 | flat stale | both hit@1 | both stale |
|---|---|---|---|---|---|
| 0.80 | 0.5 | 0.772 | 0.080 | 0.800 | 0.063 |
| 0.80 | 0.9 | 0.794 | 0.056 | 0.816 | 0.043 |
| 0.85 | 0.5 | 0.765 | 0.090 | 0.797 | 0.069 |
| 0.85 | 0.9 | 0.779 | 0.075 | 0.804 | 0.059 |
| 0.90 | 0.9 | 0.763 | 0.090 | 0.796 | 0.069 |

Quality of the candidates. This table counts all earlier same-scope pairs above the threshold, not only the 3 nearest that the API returns (3,914 writes):

| threshold | shown per write | same fact (conflict or restatement) | conflict only | recall of conflicting pairs |
|---|---|---|---|---|
| 0.80 | 1.89 | 0.684 | 0.335 | 0.564 |
| 0.85 | 1.17 | 0.907 | 0.367 | 0.383 |
| 0.90 | 0.73 | 0.991 | 0.303 | 0.197 |

Embeddings find "the same fact" well but cannot tell a conflict from a restatement (at most 37% of shown pairs are conflicts). That is why the store shows candidates and the writer decides; it never merges.

### 3.3 Calibration of the belief score

Same-setting memories in the top 5, test seeds:

| scorer | feedback | ECE (15 bins) | Brier | AUC |
|---|---|---|---|---|
| stated level only | same agent | 0.128 | 0.239 | 0.505 |
| Beta posterior (what the server returns) | same agent | 0.114 | 0.215 | 0.634 |
| Beta + Platt (fit on train seeds) | same agent | 0.035 | 0.203 | 0.634 |
| stated level only | noisy agent | 0.128 | 0.239 | 0.504 |
| Beta posterior | noisy agent | 0.115 | 0.232 | 0.543 |
| Beta + Platt (fit on train seeds) | noisy agent | 0.092 | 0.237 | 0.543 |

Reliability, same agent. Raw Beta posterior (as the server returns it):

| bin | n | mean score | observed right |
|---|---|---|---|
| 0.4-0.6 | 1,716 | 0.530 | 0.260 |
| 0.6-0.8 | 1,308 | 0.689 | 0.674 |
| 0.8-1.0 | 13,715 | 0.808 | 0.717 |

Beta + Platt:

| bin | n | mean score | observed right |
|---|---|---|---|
| 0.2-0.4 | 1,736 | 0.273 | 0.262 |
| 0.4-0.6 | 1,173 | 0.487 | 0.674 |
| 0.6-0.8 | 12,227 | 0.707 | 0.692 |
| 0.8-1.0 | 1,603 | 0.832 | 0.910 |

The stated level carries no information (AUC 0.50). The evidence adds some (0.63). A calibrated number needs a calibrator fitted on labelled feedback, and the fit does not survive a change in agent behaviour. So the server returns the raw posterior as `belief` and calls it uncalibrated.

### 3.4 LongMemEval_S: no feedback, so strength is recency

222 questions: all 72 knowledge-update questions (abstention excluded) and 30 of each other type, about 19,000 session memories, 10 local owners. Every search ran in hybrid mode. Latency through the local API: p50 136 ms, p95 241 ms.

| variant | hit@1 | recall@5 | recall@10 | MRR |
|---|---|---|---|---|
| flat | 0.730 | 0.923 | 0.959 | 0.809 |
| recency w=0.25 | 0.725 | 0.923 | 0.959 | 0.807 |
| recency w=0.5 (the shipped strength weight) | 0.721 | 0.919 | 0.959 | 0.802 |
| recency w=1 | 0.671 | 0.896 | 0.959 | 0.771 |
| recency w=2 | 0.500 | 0.847 | 0.946 | 0.654 |

Knowledge-update questions, where the newest answer session holds the current value:

| variant | newest at rank 1 | newest in top 5 | newest ranked above the older one |
|---|---|---|---|
| flat | 0.486 | 0.972 | 38/72 = 0.528 |
| recency w=0.25 | 0.681 | 0.972 | 54/72 = 0.750 |
| recency w=0.5 | 0.736 | 0.972 | 58/72 = 0.806 |
| recency w=1 | 0.806 | 0.944 | 65/72 = 0.903 |

Recall@5 by type barely moves at w=0.5 (single-session-user 1.000 → 0.933, single-session-assistant 0.767 → 0.800; the others are unchanged).

What this shows: flat search finds the evidence (recall@10 0.959) but puts the stale session first half the time. A bounded recency term fixes most of that order (0.528 → 0.806) at a cost of 0.9 hit@1 points on all questions. Large weights do harm. This is retrieval only: we did not run an LLM reader.


### 3.5 One real store (private; aggregates only)

About 1,000 memories that one person's agents wrote over a month, loaded only into the local API. Nothing from this store left the machine, and no LLM saw it. There is no use log and no correctness label, so this suite checks only "no harm", latency and conflict-candidate rates.

- **Stated confidence:** 95% `high`, 5% `medium`. The enum carries almost no information, which agrees with the LLM-calibration literature above.
- **Known-item search:** 1,026 queries of 4 random content words from a memory's body. All ran in hybrid mode, at p50 72 ms and p95 138 ms.

| variant | hit@1 | recall@5 | recall@10 | MRR |
|---|---|---|---|---|
| flat | 0.293 | 0.717 | 0.865 | 0.476 |
| recency w=0.5 | 0.270 | 0.697 | 0.860 | 0.458 |
| recency w=1 | 0.240 | 0.671 | 0.846 | 0.429 |

With no use signal, `on` acts as recency. It costs 2.3 hit@1 points on queries that look for an old memory by its words. This is the cost side of the LongMemEval gain, and it is one more reason for the staged flag.

- **Conflict candidates:** pairs in the same repo with different text above a cosine threshold. At 0.80 there are 552 pairs, about 1.07 per memory. At 0.95 there are still 533, so most are near-copies. Two of the three known supersede pairs are above 0.80, and one is above 0.85. Similar-on-write would show about one memory per write in this store, and most of them would be restatements.


### 3.6 Similar on write through the real server

On a local API with `collect`, 40 writes give a new value to settings of test world 4. Their `similar` lists were compared with the exact top 3 by cosine over the stored vectors.

- **Match:** the lists match in 36 of 40 writes. One list was shorter than the truth. We did not inspect the other three differences.
- **Fix found by this check:** before it, 2 of 40 lists were shorter. The query used the HNSW index, which takes about 40 nearest neighbours over all owners and filters by owner afterwards. The query now orders by the similarity expression, so it is an exact scan of the owner's rows.
- **Latency:** a write that returns `similar` takes p50 45 ms and p95 88 ms, embedding included.
- **Acting on `similar`:** the writer sends the same write again with `supersedes`. A test covers this with and without an origin.

Note: the vector leg of hybrid search has the same pattern (`ORDER BY distance LIMIT pool` with an owner filter). On a server with many users, an HNSW scan can return few of one user's rows. This behaviour is pre-existing and out of scope here; pgvector 0.8 `hnsw.iterative_scan` would fix it.

### 3.7 Server parity and latency

The parity suite runs on a local API with `MEMORY_STRENGTH=on` and no feedback. For each of the 2,922 distinct queries of the test worlds, it compares the server's top 10 with the eval's re-rank of the flat top 50: **2,922/2,922 are equal**. With `collect`, which keeps search flat, only 1/2,922 is equal. This shows that the check can detect a difference.

Search latency on the same queries is p50 49 ms and p95 321 ms with `on`, and p50 48 ms and p95 about 450 ms with flat search. The re-rank adds no latency that we can measure; the p95 is noise from the local machine. Every suite stayed far inside the 4 s context budget.

**Storage.** Four columns (about 24 bytes) per memory, against a median memory of about 3.5 KB including its vector (under 1%). Each `confirmed` or `wrong` vote adds one `memory_feedback` row of about 60 bytes, at most one per memory, signal and credential. A `useful` signal only updates two columns, so repeated use adds no storage (owner decision, 2026-11-24).


## 4. Recommendation

1. **Now** (owner decision, 2026-11-24): `MEMORY_STRENGTH=collect` on every environment, as the default in `docker-compose.yml` (similar-on-write, feedback and signals; search stays flat). Similar-on-write needs embeddings, so it works only where `OLLAMA_URL` is set. Teach the agents (hub, MCP clients) to send `useful` for a memory they used and `wrong` for one they found false, and to supersede what `similar` shows.
2. **After about 500 labelled signals**: rerun this eval on real feedback (hit@1 on queries whose answer the agent marked). Turn the re-rank on if it beats flat by more than the seed noise (about 2 points). Fit Platt on the labels before any client treats `belief` as a probability.
3. **Keep**: flat store, hybrid RRF, verbatim compaction, explicit supersede.
4. **Next research**:
   - Client-side citation reports, as an automatic `useful`.
   - Bi-temporal valid time on supersede.
   - An NLI check on similar-on-write candidates, if writers ignore them or if near-copies drown them.

## 5. Risks

| Risk | Guard |
|---|---|
| Rich-get-richer: a strong memory ranks higher, gets used more, gets stronger. | Search is never counted as use; the re-rank is bounded and keeps the best relevance match. |
| Wrong or poisoned feedback pushes a false memory up. | One vote per credential; only the owner's credentials vote; votes reset on new text; belief alone is weak under noise, so it has the smaller weight. |
| A false precision number misleads clients. | The field is `belief`, documented as uncalibrated. |
| Similar-on-write slows writes. | One embedding call with a 1 s timeout, best effort, Postgres only. |
| Recency (strength without feedback) hides old memories that users look for by their words: -2.3 hit@1 on the real store, -0.9 on LongMemEval. | The staged flag keeps search flat in `collect`. Limit recency to kinds that change before you turn on `on`. |
| The simulator flatters the method. | Weights chosen on train seeds, reported on held-out seeds, with a noisy-feedback shift, a need-distribution sweep and paired per-seed deltas. |
| Writes slow down as one owner's store grows (pre-existing). | Each write takes an owner lock and sums the owner's usage; about 0.6 s per write at 20,000 memories locally. Out of scope here; a cached usage counter would fix it. |

## 6. What we did not check

- Real feedback: no client sends it yet, so every re-rank gain is simulated.
- LongMemEval and LoCoMo QA accuracy with an LLM reader: we measured retrieval only, without LLM calls. LoCoMo was not run at all: its licence is non-commercial, and LongMemEval covers knowledge updates better.
- LongMemEval on all 470 questions: we ran a stratified 222, because local embedding took about 15 texts per second.
- Multi-user sharing: feedback is owner-only.
- The SQLite path of similar-on-write (it is off there by design).
- The re-rank tie rule and the single-slot rule changed after the eval runs (from the second cross-review). The eval used limits of 10 and 50, where the change matters only at exact relevance ties, so we did not rerun it.
- Concurrent feedback and compaction: the owner lock orders them, but no test runs them in parallel.
- Measurements older than the last fixes: the parity check (2,922/2,922) and the similar check (36/40) ran before the re-rank tie rules changed and before similar-on-write stored its vector. In the similar check, the truth set included earlier probes that had no vector yet, so 36/40 probably understates the match.
- CI does not run the Postgres tests (`TEST_POSTGRES_DSN` is not set there); they ran on a local pgvector database.
- Whether `memory_feedback` rows count toward the quota: they do not today.

## 7. Suggested go-draw diagrams for the blog post

1. **Flat store, three signals**: one memory row with relevance (search), strength (use) and belief (evidence) feeding the re-rank.
2. **The strength curve**: ACT-R activation over 60 days for a memory used once, used three times in one hour, and used three times a week apart (spacing).
3. **The belief counter**: a Beta prior from the stated level, moved by confirmed and wrong votes; one vote per credential.
4. **Where the errors are**: stacked bars of top-1 right / stale / wrong / other for flat, re-rank, similar-on-write and oracle supersede.
5. **Similar on write**: sequence diagram — agent writes, store embeds, returns 3 close memories, agent supersedes the outdated one.
6. **Re-rank, never hide**: candidate pool from RRF, two extra legs, the top relevance match pinned in the results.
7. **Calibration**: reliability diagram of the raw belief and of Platt-scaled belief, with the noisy-agent shift.
8. **What we skip**: a small table-diagram of NLI, truth discovery, FSRS and graphs, each with the reason.
