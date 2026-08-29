//go:build linux

package main

import (
	"reflect"
	"testing"
)

func TestInjectFrontSecretEnv(t *testing.T) {
	env := map[string]string{
		frontRouteKeyEnv: "route-secret-from-env",
		frontUsernameEnv: "user-from-env",
		frontPasswordEnv: "password-from-env",
	}
	getenv := func(k string) string { return env[k] }
	got := injectFrontSecretEnv([]string{"wbd-faketcp-mux", "server", "--listen", "192.0.2.1:443"}, getenv)
	want := []string{
		"wbd-faketcp-mux", "server", "--listen", "192.0.2.1:443",
		"--front-route-key", "route-secret-from-env",
		"--username", "user-from-env",
		"--password", "password-from-env",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args=%q want %q", got, want)
	}
}

func TestInjectFrontSecretEnvExplicitCLIWins(t *testing.T) {
	env := map[string]string{
		frontRouteKeyEnv: "env-route",
		frontUsernameEnv: "env-user",
		frontPasswordEnv: "env-password",
	}
	getenv := func(k string) string { return env[k] }
	in := []string{
		"wbd-faketcp-mux", "server",
		"--front-route-key=cli-route",
		"--username", "cli-user",
		"--password=cli-password",
	}
	got := injectFrontSecretEnv(in, getenv)
	if !reflect.DeepEqual(got, in) {
		t.Fatalf("explicit CLI changed: got %q want %q", got, in)
	}
}

func TestInjectFrontSecretEnvDoesNotAffectOtherMode(t *testing.T) {
	getenv := func(string) string { return "secret" }
	in := []string{"wbd-faketcp-mux", "help"}
	got := injectFrontSecretEnv(in, getenv)
	if !reflect.DeepEqual(got, in) {
		t.Fatalf("non-server args changed: got %q want %q", got, in)
	}
}
