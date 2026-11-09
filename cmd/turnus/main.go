package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/sanifhimani/turnus/internal/check"
	"github.com/sanifhimani/turnus/internal/discover"
)

func main() {
	jobs, errs := discover.Discover(context.Background(), discover.Options{})
	check.Run(jobs, check.Local())
	for _, err := range errs {
		fmt.Fprintln(os.Stderr, "turnus:", err)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(jobs); err != nil {
		fmt.Fprintln(os.Stderr, "turnus:", err)
		os.Exit(1)
	}
}
