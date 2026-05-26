package memory

import "time"

// Chunk is the unit of retrieval. Phase 0 uses Content, Embedding, Metadata,
// Source, and Tenant; the freshness fields are present so Phase 2 can add
// behavior without a schema migration ordering nightmare, but they are
// nullable and unused this phase.
type Chunk struct {
	ID           string
	Content      string
	Embedding    []float32
	Metadata     map[string]any
	Source       string
	Tenant       string // empty in single-tenant deployments
	PublishedAt  time.Time
	ValidFrom    *time.Time
	ValidTo      *time.Time
	SupersededBy *string
	CreatedAt    time.Time
}

type RetrievedChunk struct {
	Chunk
	Score float64
}

type Query struct {
	Text    string
	K       int
	AsOf    *time.Time
	Filters map[string]any
}

type RawDocument struct {
	ID       string
	Source   string
	Text     string
	Metadata map[string]any
}

type ParsedDocument struct {
	ID          string
	Source      string
	Text        string
	Metadata    map[string]any
	PublishedAt time.Time
}

type PromptContext struct {
	System    string
	User      string
	SourceIDs []string
}

type Answer struct {
	Text      string
	Citations []string
}
