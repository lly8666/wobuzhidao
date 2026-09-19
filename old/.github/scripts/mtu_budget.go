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
	fecMode := flag.String("fec", "off", "off or fixed 20:x profile")
	flag.Parse()

	fecEnabled := false
	switch *fecMode {
	case "off":
	case "20:4", "20:8", "20:10", "20:12", "20:16", "20:20":
		fecEnabled = true
	default:
		fmt.Fprintln(os.Stderr, "unsupported FEC mode")
		os.Exit(2)
	}

	b, err := pathmtu.Derive(*connection, pathmtu.Features{FEC: fecEnabled, Game: true})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := json.NewEncoder(os.Stdout).Encode(b); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
