// Package brain manages the lifecycle of a local llama.cpp "brain" process
// (start/stop/switch/auto-wake/idle-unload). The registry maps a model name to
// the binary + gguf + tuning args; the Manager composes the full argv and the
// shared --host/--port (see manager.go). All process side effects are reached
// only through policy.GuardedBrain — this package holds the raw mechanism.
package brain

import (
	"encoding/json"
	"fmt"
	"os"
)

// Model is one launchable brain configuration.
type Model struct {
	Name   string   `json:"name"`
	Binary string   `json:"binary"` // llama-server path
	GGUF   string   `json:"gguf"`   // model file (-m)
	Args   []string `json:"args"`   // shared tuning flags (ctx-size, ngl, batch, …)
}

// Registry is the loaded set of named models.
type Registry struct {
	models map[string]Model
}

type registryFile struct {
	Models []Model `json:"models"`
}

// LoadRegistry reads a JSON model registry from path.
func LoadRegistry(path string) (*Registry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read brain models %q: %w", path, err)
	}
	var rf registryFile
	if err := json.Unmarshal(data, &rf); err != nil {
		return nil, fmt.Errorf("parse brain models %q: %w", path, err)
	}
	m := make(map[string]Model, len(rf.Models))
	for _, model := range rf.Models {
		if model.Name == "" {
			return nil, fmt.Errorf("brain model with empty name in %q", path)
		}
		m[model.Name] = model
	}
	return &Registry{models: m}, nil
}

// Get returns the named model or an error if it is not registered.
func (r *Registry) Get(name string) (Model, error) {
	model, ok := r.models[name]
	if !ok {
		return Model{}, fmt.Errorf("unknown brain model %q", name)
	}
	return model, nil
}

// Names lists the registered model names.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.models))
	for n := range r.models {
		names = append(names, n)
	}
	return names
}
