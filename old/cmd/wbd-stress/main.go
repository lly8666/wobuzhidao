package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/lly8666/wobuzhidao/internal/benchmark"
)

func main() {
	rows, err := benchmark.StressMatrix()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(struct {
		Schema string `json:"schema"`
		Rows   any    `json:"rows"`
	}{Schema: "wbd-stress-report/v1", Rows: rows}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
