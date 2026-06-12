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
			// Brain lifecycle skill: automatic actions allow (non-blocking, audited);
			// manual destructive actions ask/OOB.
			{ID: "allow-brain-wake", Contains: "brain wake", Effect: domain.EffectAllow, Reason: "auto-wake / manual start of the local brain"},
			{ID: "allow-brain-status", Contains: "brain status", Effect: domain.EffectAllow, Reason: "brain status is a read"},
			{ID: "allow-brain-autounload", Contains: "brain autounload", Effect: domain.EffectAllow, Reason: "idle-TTL unload of the local brain"},
			{ID: "ask-brain-switch", Contains: "brain switch", Effect: domain.EffectAsk, Channel: domain.ChannelOOB, Reason: "manual brain model switch requires confirmation"},
			{ID: "ask-brain-stop", Contains: "brain stop", Effect: domain.EffectAsk, Channel: domain.ChannelOOB, Reason: "manual brain stop requires confirmation"},
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
