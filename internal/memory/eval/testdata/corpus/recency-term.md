---
published_at: 2024-03-15T00:00:00Z
---
# Recency term

The hybrid SQL adds a recency boost to the RRF score:
`0.1 * exp(-Δt / 2592000)` where Δt is seconds since published_at and
2592000 is the 30-day half-life. The 0.1 weight and 30-day half-life
are tuning knobs Phase 3 may revise against the eval set.
