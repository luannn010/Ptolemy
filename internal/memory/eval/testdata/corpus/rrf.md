---
published_at: 2024-01-15T00:00:00Z
---
# RRF: Reciprocal Rank Fusion

Vector distance and BM25 scores live on different scales. RRF combines
ranks instead of raw scores: `rrf_score(chunk) = Σ 1 / (C + rank_in_list)`.
The constant C is 60. A chunk appearing in only one list still scores
from that list. No normalization, no per-arm weights.
