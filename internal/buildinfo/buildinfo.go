package buildinfo

import (
	"fmt"
	"runtime"
	"strings"
)

var (
	Version   = "dev"
	SourceSHA = "unknown"
)

func String(program string) string {
	program = strings.TrimSpace(program)
	if program == "" {
		program = "wbd"
	}
	return fmt.Sprintf("%s version=%s source_sha=%s go=%s os=%s arch=%s",
		program, Version, SourceSHA, runtime.Version(), runtime.GOOS, runtime.GOARCH)
}
