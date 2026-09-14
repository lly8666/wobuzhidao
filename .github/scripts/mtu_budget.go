package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/lly8666/wobuzhidao/internal/pathmtu"
)

func main() {
	connection := flag.Int("connection-mtu", 1420, "operator-visible connection MTU")
	fecMode := flag.String("fec", "off", "off or 20:20")
	flag.Parse()
	if *fecMode != "off" && *fecMode != "20:20" {
		fmt.Fprintln(os.Stderr, "unsupported FEC mode")
		os.Exit(2)
	}
	b, err := pathmtu.Derive(*connection, pathmtu.Features{FEC: *fecMode == "20:20", Game: true})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := json.NewEncoder(os.Stdout).Encode(b); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
