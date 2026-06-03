# Supply Chain Proof of Absence: Publication Readiness

## Status: Zenodo-ready

The paper has formal model, soundness theorem, two case studies (clearly marked
as reconstructed), 200-package FP evaluation, CI integration protocol, threats
to validity, reproducibility section, prior work citation, and consistent
section numbering.

## What's needed for peer review (beyond Zenodo)

### 4. End-to-end demo on a reproducible attack
Since original artifacts are scrubbed, create a synthetic attack fixture:
- Clean package with stream utilities (mimics event-stream v3.3.3)
- Compromised version with injected dependency that creates capability path to network
- Ship as test fixtures in the repo so reviewers can reproduce
- Run `prove-absent` on clean (succeeds), `prove` on compromised (detects path)
- This replaces the "we would have detected it" claim with "here is the detection"

Estimated effort: 1 session

### 5. Comparison with Socket.dev on shared corpus
- Run Socket.dev on the same 200 packages
- Compare FP rates, detection latency, proof capability
- Socket.dev is the closest competitor; direct comparison strengthens the paper

Estimated effort: 0.5 session (if Socket.dev API is accessible)

### 6. Expand evaluation corpus
- Add 100 more packages (50 npm, 50 PyPI) as held-out validation
- The current 200-package corpus was used for both tuning and evaluation
- Threats to validity notes this; fixing it removes the caveat

Estimated effort: 0.5 session

### 7. Formal proof of soundness -- DONE (Section 3.5)

## Target venues

| Venue | Fit | Deadline |
|-------|-----|----------|
| USENIX Security 2027 | Security audience, systems focus | ~Feb 2027 |
| IEEE S&P 2027 | Top security venue, formal methods welcome | ~Dec 2026 |
| CCS 2026 | Applied crypto + systems | ~May 2026 (may have passed) |
| NDSS 2027 | Network/distributed security | ~Jun 2026 |
| SCORED (co-located with CCS) | Supply chain security workshop | ~Aug 2026 |
| AsiaCCS 2027 | Applied security, shorter papers accepted | ~Dec 2026 |

**Recommendation:** SCORED workshop (supply chain focused, shorter format matches
current length) or Zenodo preprint + USENIX Security full paper with the synthetic
demo and expanded corpus.

## Immediate next step

Publish on Zenodo alongside the shared intelligence layer paper. Two preprints,
two DOIs, both citable in the blog and README.
