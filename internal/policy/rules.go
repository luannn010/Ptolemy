package policy

import (
	"encoding/json"
	"os"

	"github.com/luannn010/ptolemy/internal/domain"
)

type Rule struct {
	ID       string         `json:"id"`
	Contains string         `json:"contains"`
	Effect   domain.Effect  `json:"effect"`
	Channel  domain.Channel `json:"channel,omitempty"`
	Reason   string         `json:"reason"`
}

type Ruleset struct {
	Rules []Rule `json:"rules"`
}

func DefaultRuleset() Ruleset {
	return Ruleset{
		Rules: []Rule{
			{ID: "deny-policy-write", Contains: ".ptolemy/policy.json", Effect: domain.EffectDeny, Reason: "policy file is protected"},
			{ID: "deny-secret-cmd", Contains: ".env", Effect: domain.EffectDeny, Reason: "secret file access denied"},
			{ID: "deny-destructive", Contains: "git push --force", Effect: domain.EffectDeny, Reason: "force push denied"},
			{ID: "deny-destructive-rm", Contains: "rm -rf", Effect: domain.EffectDeny, Reason: "destructive deletion denied"},
			{ID: "ask-push-cmd", Contains: "git push", Effect: domain.EffectAsk, Channel: domain.ChannelOOB, Reason: "git push requires confirmation"},
			{ID: "ask-network", Contains: "curl http", Effect: domain.EffectAsk, Channel: domain.ChannelOOB, Reason: "network download requires confirmation"},
			{ID: "allow-build-test", Contains: "go test", Effect: domain.EffectAllow, Reason: "build/test command allowed"},
		},
	}
}

func LoadRuleset(path string) Ruleset {
	data, err := os.ReadFile(path)
	if err != nil {
		return DefaultRuleset()
	}
	var rs Ruleset
	if err := json.Unmarshal(data, &rs); err != nil || len(rs.Rules) == 0 {
		return DefaultRuleset()
	}
	return rs
}
