package memory

import "math"

// decayScore computes a memory's retention score. Global-scope or pinned rows
// are decay-immune (always 1.0) — enforced here AND by the WHERE clause in the
// archive sweep, so immunity does not depend on a tunable.
//
// For decaying ('project', not pinned) rows:
//
//	score = importance * exp( -lambda * daysSinceAccess / (1 + accessCount) )
//
// Reinforcement (high accessCount) flattens the curve so frequently-used
// memories persist. The archive sweep inlines this same formula in SQL;
// TestDecayScore_GoMatchesSQL guards the two against drift.
func decayScore(importance float64, scope string, pinned bool, accessCount int, daysSinceAccess, lambda float64) float64 {
	if scope == "global" || pinned {
		return 1.0
	}
	return importance * math.Exp(-lambda*daysSinceAccess/(1+float64(accessCount)))
}
