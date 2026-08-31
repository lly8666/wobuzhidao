//go:build linux

package main

import "os"

const (
	frontRouteKeyEnv = "WBD_FRONT_ROUTE_KEY"
	frontUsernameEnv = "WBD_FRONT_USERNAME"
	frontPasswordEnv = "WBD_FRONT_PASSWORD"
)

// init injects protected runtime secret values into Go's process-local os.Args
// only after exec. The kernel-visible argv used by ps/systemctl therefore keeps
// route keys and account credentials out of the command line. CLI flags remain
// supported for tests/legacy invocations, and explicit CLI values win.
//
// The official V3 manager reads /etc/wbd/server.env (0600) and supplies these
// three environment variables only to the WBD guard/mux child process. This is
// not a privilege boundary against root, but it removes the previously observed
// systemctl-status argv disclosure while keeping the config file root-readable.
func init() {
	os.Args = injectFrontSecretEnv(os.Args, os.Getenv)
}

func injectFrontSecretEnv(args []string, getenv func(string) string) []string {
	out := append([]string(nil), args...)
	if len(out) < 2 || out[1] != "server" {
		return out
	}
	present := func(name string) bool {
		for i := 2; i < len(out); i++ {
			if out[i] == name || len(out[i]) > len(name) && out[i][:len(name)+1] == name+"=" {
				return true
			}
		}
		return false
	}
	appendSecret := func(flagName, envName string) {
		if present(flagName) {
			return
		}
		if value := getenv(envName); value != "" {
			out = append(out, flagName, value)
		}
	}
	appendSecret("--front-route-key", frontRouteKeyEnv)
	appendSecret("--username", frontUsernameEnv)
	appendSecret("--password", frontPasswordEnv)
	return out
}
