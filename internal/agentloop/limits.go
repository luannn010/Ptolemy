package agentloop

type LoopLimits struct {
	MaxSteps             int
	MaxInvalidJSON       int
	MaxSameActionRetries int
	MaxNoProgressSteps   int
}

func DefaultLimits() LoopLimits {
	return LoopLimits{
		MaxSteps:             8,
		MaxInvalidJSON:       3,
		MaxSameActionRetries: 2,
		MaxNoProgressSteps:   2,
	}
}

func (l LoopLimits) WithMaxSteps(maxSteps int) LoopLimits {
	if maxSteps > 0 {
		l.MaxSteps = maxSteps
	}
	return l
}
