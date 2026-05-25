package domain

type Effect string

const (
	EffectAllow Effect = "allow"
	EffectAsk   Effect = "ask"
	EffectDeny  Effect = "deny"
)

type Channel string

const (
	ChannelNone  Channel = ""
	ChannelToken Channel = "token"
	ChannelOOB   Channel = "oob"
)

type Intent struct {
	Kind    string
	Program string
	Args    []string
	Targets []string
}

type Decision struct {
	Effect  Effect
	Channel Channel
	RuleID  string
	Reason  string
}
