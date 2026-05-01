package config

import (
	"fmt"
	"runtime"

	"github.com/luannn010/ptolemy/internal/shellcmd"
)

type Config struct {
	ServerURL      string
	ProjectName    string
	Workspace      string
	SkillsCache    string
	AllowDelete    bool
	AllowShell     bool
	Shell          string
	TimeoutSeconds int
}

func Default(projectName string, serverURL string) Config {
	if serverURL == "" {
		serverURL = "http://localhost:8080"
	}
	if projectName == "" {
		projectName = "example-project"
	}

	return Config{
		ServerURL:      serverURL,
		ProjectName:    projectName,
		Workspace:      ".",
		SkillsCache:    ".ptolemy/cache/skills",
		AllowDelete:    false,
		AllowShell:     true,
		Shell:          shellcmd.DefaultProgram(runtime.GOOS),
		TimeoutSeconds: 120,
	}
}

func (c Config) YAML() string {
	return fmt.Sprintf(
		"server_url: %q\nproject_name: %q\nworkspace: %q\nskills_cache: %q\nallow_delete: %t\nallow_shell: %t\nshell: %q\ntimeout_seconds: %d\n",
		c.ServerURL,
		c.ProjectName,
		c.Workspace,
		c.SkillsCache,
		c.AllowDelete,
		c.AllowShell,
		c.Shell,
		c.TimeoutSeconds,
	)
}
