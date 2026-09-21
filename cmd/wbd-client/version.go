package main

import (
	"flag"
	"fmt"

	"github.com/lly8666/wobuzhidao/internal/buildinfo"
)

var showVersion = flag.Bool("version", false, "print build version and source SHA")

func handleVersion() bool {
	if !*showVersion {
		return false
	}
	fmt.Println(buildinfo.String("wbd-client"))
	return true
}

func parseAndHandleVersion() bool {
	flag.Parse()
	return handleVersion()
}
