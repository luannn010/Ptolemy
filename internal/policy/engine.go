package policy

import (
	"strings"

	"github.com/luannn010/ptolemy/internal/domain"
)

type Engine struct {
	rules Ruleset
}

func NewEngine(rules Ruleset) *Engine {
	return &Engine{rules: rules}
}

func (e *Engine) Authorize(intent domain.Intent) domain.Decision {
	cmd := strings.ToLower(strings.TrimSpace(strings.Join(append([]string{intent.Program}, intent.Args...), " ")))
	if cmd == "" {
		cmd = strings.ToLower(strings.TrimSpace(strings.Join(intent.Args, " ")))
	}

	var best *domain.Decision
	for _, r := range e.rules.Rules {
		if strings.Contains(cmd, strings.ToLower(r.Contains)) {
			d := domain.Decision{Effect: r.Effect, Channel: r.Channel, RuleID: r.ID, Reason: r.Reason}
			if moreRestrictive(d.Effect, best) {
				tmp := d
				best = &tmp
			}
		}
	}
	if best != nil {
		return *best
	}
	return domain.Decision{
		Effect:  domain.EffectAsk,
		Channel: domain.ChannelOOB,
		RuleID:  "default",
		Reason:  "command requires confirmation by default",
	}
}

func moreRestrictive(next domain.Effect, current *domain.Decision) bool {
	if current == nil {
		return true
	}
	score := func(e domain.Effect) int {
		switch e {
		case domain.EffectDeny:
			return 3
		case domain.EffectAsk:
			return 2
		default:
			return 1
		}
	}
	return score(next) > score(current.Effect)
}
