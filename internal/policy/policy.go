package policy

import "strings"

type Mode string

const (
	ModeAllow Mode = "allow"
	ModeAsk   Mode = "ask"
	ModeDeny  Mode = "deny"
)

type Decision struct {
	Mode       Mode
	ActionType string
	Reason     string
}

func CheckCommand(command string) Decision {
	cmd := strings.ToLower(strings.TrimSpace(command))

	rules := []struct {
		pattern    string
		mode       Mode
		actionType string
		reason     string
	}{
		{"git push", ModeAsk, "git.push", "git push requires approval"},
		{"rm -rf", ModeAsk, "filesystem.delete_recursive", "recursive deletion requires approval"},
		{"rm -fr", ModeAsk, "filesystem.delete_recursive", "recursive deletion requires approval"},
		{"sudo rm -rf", ModeAsk, "filesystem.delete_recursive", "recursive deletion requires approval"},
		{"sudo rm -fr", ModeAsk, "filesystem.delete_recursive", "recursive deletion requires approval"},
		{"git reset --hard", ModeAsk, "git.reset_hard", "git reset --hard requires approval"},
		{"docker system prune", ModeAsk, "docker.prune", "docker system prune requires approval"},
		{"curl http", ModeAsk, "network.download", "network download requires approval"},
		{"wget http", ModeAsk, "network.download", "network download requires approval"},
		{"cat .env", ModeDeny, "secrets.read", "reading .env is denied"},
		{"cat ~/.ssh", ModeDeny, "secrets.read", "reading SSH secrets is denied"},
	}

	for _, rule := range rules {
		if strings.Contains(cmd, rule.pattern) {
			return Decision{
				Mode:       rule.mode,
				ActionType: rule.actionType,
				Reason:     rule.reason,
			}
		}
	}

	return Decision{
		Mode:       ModeAllow,
		ActionType: "command.exec",
		Reason:     "command allowed",
	}
}
