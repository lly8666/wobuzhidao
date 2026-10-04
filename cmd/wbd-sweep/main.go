package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/lly8666/wobuzhidao/internal/benchmark"
)

func main() {
	configPath := flag.String("config", "", "path to benchmark sweep JSON")
	runID := flag.String("run", "", "run one generated runnable case by exact id")
	timeout := flag.Duration("timeout", 60*time.Second, "timeout for -run")
	flag.Parse()
	if *configPath == "" {
		fmt.Fprintln(os.Stderr, "-config is required")
		os.Exit(2)
	}
	data, err := os.ReadFile(*configPath)
	fatal(err)
	spec, err := benchmark.ParseSweepJSON(data)
	fatal(err)
	cases, err := benchmark.ExpandSweep(spec)
	fatal(err)
	if *runID == "" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		fatal(enc.Encode(cases))
		return
	}
	var selected *benchmark.ExperimentCase
	for i := range cases {
		if cases[i].ID == *runID {
			selected = &cases[i]
			break
		}
	}
	if selected == nil {
		fatal(fmt.Errorf("case %q not found", *runID))
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	obs, err := benchmark.RunExperimentCase(ctx, *selected)
	fatal(err)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	fatal(enc.Encode(obs))
}

func fatal(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
