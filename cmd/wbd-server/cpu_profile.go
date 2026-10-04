package main

import (
	"github.com/lly8666/wobuzhidao/internal/qualificationdiag"
	"log"
	"os"
)

func startQualificationCPUProfile() func() {
	stop, err := qualificationdiag.StartCPUProfile(os.Getenv("WBD_QUALIFICATION_CPU_PROFILE"))
	if err != nil {
		log.Fatal("cannot start qualification CPU profile")
	}
	return stop
}
