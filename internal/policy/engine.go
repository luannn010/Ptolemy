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

// Authorize evaluates an Intent against the ruleset and returns the
// most-restrictive Decision. It defends against three classes of bypass:
//   - whitespace tricks (double spaces, tabs, newlines) via canonicalization
//   - compound commands (a && b ; c | d) via SplitCompound, evaluating each
//     sub-command and keeping the worst verdict
//   - target smuggling — Intent.Targets are appended to the matched string so
//     a `file.read` intent against `.env` is still caught by `deny-secret-cmd`
//
// When no rule matches, the fail-safe default is Ask/OOB so the agent cannot
// proceed without out-of-band human approval.
func (e *Engine) Authorize(intent domain.Intent) domain.Decision {
	raw := strings.TrimSpace(intent.Program + " " + strings.Join(intent.Args, " ") + " " + strings.Join(intent.Targets, " "))
	raw = strings.ToLower(raw)

	segments := SplitCompound(raw)
	if len(segments) == 0 {
		segments = []string{collapseSpaces(raw)}
	}

	var best *domain.Decision
	for _, seg := range segments {
		for _, r := range e.rules.Rules {
			needle := collapseSpaces(strings.ToLower(strings.TrimSpace(r.Contains)))
			if needle == "" {
				continue
			}
			if !strings.Contains(seg, needle) {
				continue
			}
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
