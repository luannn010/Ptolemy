APP_NAME=workerd
BIN_DIR=bin

run:
	go run ./cmd/workerd

build:
	mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/workerd ./cmd/workerd
	go build -o $(BIN_DIR)/ptolemy-mcp ./cmd/ptolemy-mcp
	go build -o $(BIN_DIR)/policy-demo ./cmd/policy-demo
	go build -o $(BIN_DIR)/memory-demo ./cmd/memory-demo
	go build -o $(BIN_DIR)/memory-eval ./cmd/memory-eval

test:
	go test -p 1 ./...

test-integration:
	go test -tags=integration ./...

fmt:
	go fmt ./...

tidy:
	go mod tidy

# Phase 0 memory smoke test. Overrides RAG_CHUNK_SIZE_TOKENS for the run so
# small embedding servers (e.g. llama.cpp with --batch-size 64) don't reject
# the input. Everything else (DATABASE_URL, EMBEDDING_*, BRAIN_*) is read
# from .env via cmd/memory-demo's godotenv autoload.
SMOKE_TEST_CHUNK_SIZE ?= 50
SMOKE_TEST_DOC        ?= /tmp/ptolemy-smoke.txt
SMOKE_TEST_DOC_ID     ?= smoke-doc
SMOKE_TEST_QUESTION   ?= What is Ptolemy?

smoke-memory: build
	@if [ ! -f $(SMOKE_TEST_DOC) ]; then \
	  printf "Ptolemy is a Go-based agent runtime project being rebuilt clean-room as v2.\nIt uses a policy harness to gate every side-effecting operation (shellcmd, fileops, gitops, worktrees) behind hybrid approvals: in-band tokens for low-risk commands and out-of-band approval for high-risk ones.\nThe memory module adds Retrieval-Augmented Generation on PostgreSQL with pgvector for dense semantic search.\n" > $(SMOKE_TEST_DOC); \
	fi
	@echo "--- ingest ($(SMOKE_TEST_DOC)) ---"
	RAG_CHUNK_SIZE_TOKENS=$(SMOKE_TEST_CHUNK_SIZE) RAG_CHUNK_OVERLAP_TOKENS=10 \
	  $(BIN_DIR)/memory-demo ingest $(SMOKE_TEST_DOC_ID) $(SMOKE_TEST_DOC)
	@echo
	@echo "--- ask ($(SMOKE_TEST_QUESTION)) ---"
	RAG_CHUNK_SIZE_TOKENS=$(SMOKE_TEST_CHUNK_SIZE) RAG_CHUNK_OVERLAP_TOKENS=10 \
	  $(BIN_DIR)/memory-demo ask "$(SMOKE_TEST_QUESTION)"

# Phase 6a capture smoke: runs the REAL BRAIN_* extractor against a sample
# exchange and logs the extracted entries. Requires .env (BRAIN_*).
smoke-capture:
	@set -a; . ./.env; set +a; \
	  go test -p 1 -tags=smoke -run TestExtractorSmoke ./internal/memory/ -v

# Phase 3 memory eval. RAG_FIXTURE_DIR points the binary at the frozen
# fixture corpus under internal/memory/eval/testdata/corpus/; eval.LoadFixtureCorpus
# enumerates the dir and the orchestrator's ingest path chunks/embeds/upserts.
# EVAL_CHUNK_SIZE=20 keeps chunked output under the llama.cpp embedding server's
# 64-token batch ceiling on dense markdown fixtures.
EVAL_SEED         ?= internal/memory/eval/testdata/seed.json
EVAL_FIXTURE_DIR  ?= internal/memory/eval/testdata/corpus
EVAL_CHUNK_SIZE   ?= 20

eval-memory: build
	RAG_FIXTURE_DIR=$(EVAL_FIXTURE_DIR) \
	RAG_CHUNK_SIZE_TOKENS=$(EVAL_CHUNK_SIZE) RAG_CHUNK_OVERLAP_TOKENS=10 \
	  $(BIN_DIR)/memory-eval -seed $(EVAL_SEED)

eval-memory-sweep: build
	RAG_FIXTURE_DIR=$(EVAL_FIXTURE_DIR) \
	RAG_CHUNK_SIZE_TOKENS=$(EVAL_CHUNK_SIZE) RAG_CHUNK_OVERLAP_TOKENS=10 \
	  $(BIN_DIR)/memory-eval -seed $(EVAL_SEED) -sweep

eval-memory-dedup: build
	RAG_FIXTURE_DIR=$(EVAL_FIXTURE_DIR) \
	  $(BIN_DIR)/memory-eval -seed $(EVAL_SEED) -dedup
