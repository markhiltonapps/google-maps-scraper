// Command sitebrief converts a scraper results CSV (the "Full data" download)
// into a clean website brief CSV with a ready-to-paste AI website prompt.
//
//	go run ./cmd/sitebrief results.csv briefs.csv
package main

import (
	"fmt"
	"os"

	"github.com/gosom/google-maps-scraper/internal/sitebrief"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: sitebrief <results.csv> <briefs.csv>")
		os.Exit(2)
	}

	if err := run(os.Args[1], os.Args[2]); err != nil {
		fmt.Fprintln(os.Stderr, "sitebrief:", err)
		os.Exit(1)
	}
}

func run(inPath, outPath string) error {
	in, err := os.Open(inPath) //nolint:gosec // the path is the command-line argument
	if err != nil {
		return err
	}
	defer in.Close()

	briefs, err := sitebrief.Build(in)
	if err != nil {
		return err
	}

	out, err := os.Create(outPath) //nolint:gosec // the path is the command-line argument
	if err != nil {
		return err
	}

	if err := sitebrief.WriteCSV(out, briefs); err != nil {
		_ = out.Close()

		return err
	}

	if err := out.Close(); err != nil {
		return err
	}

	fmt.Printf("wrote %d businesses to %s\n", len(briefs), outPath)

	return nil
}
