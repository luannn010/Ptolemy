package main

import (
	"context"
	"fmt"
	"os"

	"github.com/luannn010/ptolemy/internal/cli"
)

var version = "dev"

func main() {
	if err := cli.Run(context.Background(), cli.Config{Version: version}); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
