package main

import (
	"context"
	"fmt"
	"os"

	"github.com/luannn010/ptolemy/internal/memory"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}
	cfg, err := memory.LoadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		os.Exit(2)
	}
	ctx := context.Background()
	orch, conn, err := memory.NewModule(ctx, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "module:", err)
		os.Exit(2)
	}
	defer conn.Close(ctx)

	switch os.Args[1] {
	case "ingest":
		if len(os.Args) != 4 {
			usage()
			os.Exit(1)
		}
		id := os.Args[2]
		path := os.Args[3]
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, "read:", err)
			os.Exit(2)
		}
		if err := orch.Ingest(ctx, memory.RawDocument{ID: id, Source: path, Text: string(data)}); err != nil {
			fmt.Fprintln(os.Stderr, "ingest:", err)
			os.Exit(2)
		}
		fmt.Println("ingested:", id)
	case "ask":
		if len(os.Args) != 3 {
			usage()
			os.Exit(1)
		}
		ans, err := orch.Answer(ctx, memory.Query{Text: os.Args[2], K: 5})
		if err != nil {
			fmt.Fprintln(os.Stderr, "answer:", err)
			os.Exit(2)
		}
		fmt.Println(ans.Text)
		if len(ans.Citations) > 0 {
			fmt.Println("\ncitations:", ans.Citations)
		}
	default:
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage:")
	fmt.Fprintln(os.Stderr, "  memory-demo ingest <doc-id> <path-to-file>")
	fmt.Fprintln(os.Stderr, "  memory-demo ask    \"<question>\"")
}
