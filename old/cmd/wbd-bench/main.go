package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/lly8666/wobuzhidao/internal/benchmark"
)

func main() {
	pretty := flag.Bool("pretty", true, "pretty-print deterministic benchmark JSON")
	flag.Parse()
	report, err := benchmark.StandardReport()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var out []byte
	if *pretty {
		out, err = json.MarshalIndent(report, "", "  ")
	} else {
		out, err = json.Marshal(report)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	_, _ = os.Stdout.Write(append(out, '\n'))
}
