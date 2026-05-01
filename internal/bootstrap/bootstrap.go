package bootstrap

import "fmt"

type Request struct {
	ServerURL   string `json:"server_url"`
	ProjectName string `json:"project_name"`
}

type Response struct {
	RootPath             string   `json:"root_path"`
	ServerURL            string   `json:"server_url"`
	ProjectName          string   `json:"project_name"`
	ContextFiles         []string `json:"context_files"`
	TaskFolders          []string `json:"task_folders"`
	MemoryFiles          []string `json:"memory_files"`
	CacheFolders         []string `json:"cache_folders"`
	ClientConfigPath     string   `json:"client_config_path"`
	ClientConfigTemplate string   `json:"client_config_template"`
}

func Build(req Request) Response {
	serverURL := req.ServerURL
	if serverURL == "" {
		serverURL = "http://localhost:8080"
	}

	projectName := req.ProjectName
	if projectName == "" {
		projectName = "example-project"
	}

	return Response{
		RootPath:    ".ptolemy",
		ServerURL:   serverURL,
		ProjectName: projectName,
		ContextFiles: []string{
			"architecture.md",
			"conventions.md",
			"workflows.md",
			"commands.md",
			"decisions.md",
		},
		TaskFolders: []string{
			"inbox",
			"process",
			"done",
			"packs",
		},
		MemoryFiles: []string{
			"codebase-map.md",
			"dependency-map.md",
			"recent-changes.md",
		},
		CacheFolders: []string{
			"skills",
		},
		ClientConfigPath:     "client.yaml",
		ClientConfigTemplate: buildClientConfigTemplate(serverURL, projectName),
	}
}

func buildClientConfigTemplate(serverURL string, projectName string) string {
	return fmt.Sprintf(
		"server_url: %q\nproject_name: %q\nworkspace: %q\nskills_cache: %q\nallow_delete: false\nallow_shell: true\nshell: %q\ntimeout_seconds: 120\n",
		serverURL,
		projectName,
		".",
		".ptolemy/cache/skills",
		"/bin/bash",
	)
}
