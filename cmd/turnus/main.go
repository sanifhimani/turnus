package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/sanifhimani/turnus/internal/discover"
)

func main() {
	jobs, errs := discover.Discover(context.Background(), discover.Options{})
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
