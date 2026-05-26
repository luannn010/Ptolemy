package main

import (
	"fmt"

	"github.com/luannn010/ptolemy/internal/domain"
	"github.com/luannn010/ptolemy/internal/policy"
)

func main() {
	engine := policy.NewEngine(policy.LoadRuleset("./.ptolemy/policy.json"))
	samples := []string{
		"go test ./...",
		"git push origin main",
		"git push --force origin main",
		"cat ./.env",
	}
	for _, cmd := range samples {
		decision := engine.Authorize(domain.Intent{
			Kind:    "command.exec",
			Program: "shell",
			Args:    []string{cmd},
		})
		fmt.Printf("%-28s => %s", cmd, decision.Effect)
		if decision.Channel != "" {
			fmt.Printf(" (%s)", decision.Channel)
		}
		fmt.Printf(" [%s]\n", decision.RuleID)
	}
}
