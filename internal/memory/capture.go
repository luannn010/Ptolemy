package memory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	factImportance = 0.7
	relImportance  = 0.5
)

type CaptureMetrics struct {
	dropped    int64
	extractErr int64
	embedErr   int64
	captured   int64
}

func (m *CaptureMetrics) Dropped() int64    { return atomic.LoadInt64(&m.dropped) }
func (m *CaptureMetrics) ExtractErr() int64 { return atomic.LoadInt64(&m.extractErr) }
func (m *CaptureMetrics) EmbedErr() int64   { return atomic.LoadInt64(&m.embedErr) }
func (m *CaptureMetrics) Captured() int64   { return atomic.LoadInt64(&m.captured) }

type PerTurnCaptureHook struct {
	extractor *Extractor
	embedder  Embedder
	store     Store
	chain     *ValidatorChain
	ch        chan Exchange
	metrics   *CaptureMetrics
	startOnce sync.Once
}

func NewCaptureHook(ex *Extractor, emb Embedder, st Store, buf int) *PerTurnCaptureHook {
	if buf <= 0 {
		buf = 256
	}
	return &PerTurnCaptureHook{
		extractor: ex,
		embedder:  emb,
		store:     st,
		chain:     NewDefaultValidatorChain(),
		ch:        make(chan Exchange, buf),
		metrics:   &CaptureMetrics{},
	}
}

func (h *PerTurnCaptureHook) Metrics() *CaptureMetrics { return h.metrics }

func (h *PerTurnCaptureHook) Start(ctx context.Context) {
	h.startOnce.Do(func() {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case ex := <-h.ch:
					if err := h.processTurn(ctx, ex); err != nil {
						log.Warn().Err(err).Msg("capture processTurn failed; dropping exchange")
					}
				}
			}
		}()
	})
}

func (h *PerTurnCaptureHook) Enqueue(ex Exchange) {
	select {
	case h.ch <- ex:
	default:
		atomic.AddInt64(&h.metrics.dropped, 1)
		log.Warn().Msg("capture channel full; dropping exchange")
	}
}

func (h *PerTurnCaptureHook) Capture(ctx context.Context, ex Exchange) error {
	return h.processTurn(ctx, ex)
}

func (h *PerTurnCaptureHook) processTurn(ctx context.Context, ex Exchange) error {
	if d := Gate(ex); d.Skip {
		log.Info().Str("stage", "gate").Str("reason", d.Reason).Msg("capture: turn skipped")
		return nil
	}
	log.Info().Str("stage", "gate").Msg("capture: turn passed gate; calling extractor LLM")

	extractStart := time.Now()
	entries, err := h.extractor.Extract(ctx, ex)
	if err != nil {
		atomic.AddInt64(&h.metrics.extractErr, 1)
		log.Error().Str("stage", "extract").Err(err).Msg("capture: extractor LLM failed")
		return err
	}
	log.Info().Str("stage", "extract").
		Int64("ms", time.Since(extractStart).Milliseconds()).
		Int("entries", len(entries)).
		Msg("capture: extractor produced atoms")
	if len(entries) == 0 {
		log.Warn().Str("stage", "extract").Msg("capture: extractor returned 0 atoms")
		return nil
	}

	entries = h.chain.Filter(entries, TurnContext{
		UserText:      ex.UserText,
		AssistantText: ex.AssistantText,
		SessionID:     ex.SessionID,
		SubjectID:     ex.SubjectID,
		ProjectID:     ex.ProjectID,
	})
	if len(entries) == 0 {
		log.Warn().Str("stage", "validate").Msg("capture: all extracted atoms rejected by validators")
		return nil
	}

	texts := make([]string, len(entries))
	for i, e := range entries {
		log.Info().Str("stage", "extract").Int("i", i).Str("perspective", e.Perspective).Str("content", e.Content).Msg("capture: atom")
		texts[i] = e.Content
	}

	embedStart := time.Now()
	vecs, err := h.embedder.Embed(ctx, texts)
	if err != nil || len(vecs) != len(entries) {
		atomic.AddInt64(&h.metrics.embedErr, 1)
		if err == nil {
			err = errors.New("embedder returned wrong vector count")
		}
		log.Error().Str("stage", "embed").Err(err).Msg("capture: embedding failed")
		return err
	}
	log.Info().Str("stage", "embed").
		Int64("ms", time.Since(embedStart).Milliseconds()).
		Int("vectors", len(vecs)).Int("dim", len(vecs[0])).
		Msg("capture: atoms embedded")

	now := time.Now().UTC()
	for i, e := range entries {
		c := h.buildChunk(ex, e, vecs[i], now)
		if c.FactSubject != nil && c.FactPredicate != nil {
			existing, found, lerr := h.store.LookupFact(ctx, *c.FactSubject, *c.FactPredicate)
			if lerr != nil {
				return lerr
			}
			if found {
				if normalizeContent(existing.Content) == normalizeContent(c.Content) {
					if err := h.store.Reinforce(ctx, []string{existing.ID}); err != nil {
						return err
					}
				} else if err := h.store.Supersede(ctx, []Chunk{c}, existing.ID); err != nil {
					return err
				}
				atomic.AddInt64(&h.metrics.captured, 1)
				continue
			}
		}
		if err := h.store.Upsert(ctx, []Chunk{c}); err != nil {
			log.Error().Str("stage", "store").Err(err).Str("id", c.ID).Msg("capture: upsert failed")
			return err
		}
		log.Info().Str("stage", "store").Str("id", c.ID).
			Str("subject", deref(c.SubjectID)).Str("project", deref(c.ProjectID)).
			Msg("capture: atom stored")
		atomic.AddInt64(&h.metrics.captured, 1)
	}
	return nil
}

func (h *PerTurnCaptureHook) buildChunk(ex Exchange, e Atom, vec []float32, now time.Time) Chunk {
	subj, sess, proj, persp := ex.SubjectID, ex.SessionID, ex.ProjectID, e.Perspective
	imp := relImportance
	c := Chunk{
		ID:          captureChunkID(ex, e),
		Content:     e.Content,
		Embedding:   vec,
		PublishedAt: now,
		Scope:       "project",
		Status:      "active",
		Perspective: &persp,
		SubjectID:   &subj,
		SessionID:   &sess,
		ProjectID:   &proj,
		Metadata:    map[string]any{"extractor_version": ExtractorVersion, "kind": "atom"},
	}
	if e.FactSubject != "" && e.FactPredicate != "" {
		fs, fp := e.FactSubject, e.FactPredicate
		c.FactSubject = &fs
		c.FactPredicate = &fp
		imp = factImportance
	}
	c.Importance = imp
	return c
}

func captureChunkID(ex Exchange, e Atom) string {
	sum := sha256.Sum256([]byte(ex.SubjectID + "|" + ex.ProjectID + "|" + e.Content))
	return "turn:" + hex.EncodeToString(sum[:])[:24]
}

func deref(s *string) string {
	if s == nil || *s == "" {
		return "<nil>"
	}
	return *s
}
