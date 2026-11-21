package scripts

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Exercise the actual Make recipe in isolation: no real token, process, or
// network request is used, including when restoring a release after local use.
func TestLocalDeployAndRollback(t *testing.T) {
	for _, failStartup := range []bool{false, true} {
		name := "success"
		if failStartup {
			name = "startup failure"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			write := func(name, body string) {
				t.Helper()
				path := filepath.Join(dir, name)
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(body), 0700); err != nil {
					t.Fatal(err)
				}
			}
			for _, name := range []string{"Makefile", "scripts/wait-release.sh", "scripts/deploy-drift.sh"} {
				data, err := os.ReadFile(filepath.Join("..", name))
				if err != nil {
					t.Fatal(err)
				}
				write(name, string(data))
			}
			binary := func(version string) string { return "#!/bin/sh\nprintf '%s\\n' 'infoe version " + version + "'\n" }
			write("installed", binary("v0.4.0"))
			write("release", binary("v0.4.0"))
			write("local", binary("v0.0.0-local"))
			write("token", "synthetic-token")
			if err := os.Chmod(filepath.Join(dir, "token"), 0600); err != nil {
				t.Fatal(err)
			}
			write("running", "")
			if failStartup {
				write("fail-start", "")
			}
			write("bin/go", `#!/bin/sh
set -eu
printf '%s\n' "$1" >> "$TEST_STATE/go-calls"
case "$1" in
 test|vet) exit 0 ;;
 build)
  while [ "$1" != -o ]; do shift; done
  cp "$TEST_STATE/local" "$2"
  ;;
 *) exit 99 ;;
esac
`)
			write("bin/screen", `#!/bin/sh
set -eu
case "$1" in
 -ls) [ -f "$TEST_STATE/running" ] && echo '123.infoe-bots (Detached)' ;;
 -S) rm -f "$TEST_STATE/running" ;;
 -L)
  if [ -f "$TEST_STATE/fail-start" ]; then rm "$TEST_STATE/fail-start"; exit 1; fi
  touch "$TEST_STATE/running"
  ;;
 *) exit 99 ;;
esac
`)
			write("bin/sleep", "#!/bin/sh\nexit 0\n")
			write("bin/gh", `#!/bin/sh
[ -f "$TEST_STATE/allow-release" ] || exit 99
echo true
`)
			write("bin/curl", `#!/bin/sh
set -eu
[ -f "$TEST_STATE/allow-release" ] || exit 99
while [ "$1" != --output ]; do shift; done
output=$2
case "$3" in
 *.sha256) (cd "$TEST_STATE" && sha256sum release | sed 's/  release$/  infoe-bots-linux-amd64/') > "$output" ;;
 *) cp "$TEST_STATE/release" "$output" ;;
esac
`)
			write("bin/git", "#!/bin/sh\nexit 1\n")
			run := func(args ...string) (string, error) {
				t.Helper()
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				args = append(args, "BINARY="+filepath.Join(dir, "installed"), "LOG_FILE="+filepath.Join(dir, "bot.log"), "DISCORD_TOKEN_FILE="+filepath.Join(dir, "token"))
				cmd := exec.CommandContext(ctx, "make", args...)
				cmd.Dir = dir
				cmd.Env = append(os.Environ(), "PATH="+filepath.Join(dir, "bin")+":"+os.Getenv("PATH"), "TEST_STATE="+dir)
				out, err := cmd.CombinedOutput()
				return string(out), err
			}
			out, err := run("deploy-local")
			if (err != nil) != failStartup {
				t.Fatalf("deploy error: %v; output: %s", err, out)
			}
			installed, err := os.ReadFile(filepath.Join(dir, "installed"))
			if err != nil {
				t.Fatal(err)
			}
			want := "v0.0.0-local"
			if failStartup {
				want = "v0.4.0"
			}
			if !strings.Contains(string(installed), want) {
				t.Fatalf("installed wrong version: %s", installed)
			}
			if _, err := os.Stat(filepath.Join(dir, "running")); err != nil {
				t.Fatal("bot was not started/restored")
			}
			calls, err := os.ReadFile(filepath.Join(dir, "go-calls"))
			if err != nil {
				t.Fatal(err)
			}
			if string(calls) != "test\nvet\nbuild\n" {
				t.Errorf("go calls = %q", calls)
			}
			if failStartup && !strings.Contains(out, "Restored the previous binary") {
				t.Error("missing rollback message")
			}
			write("allow-release", "")
			out, err = run("deploy", "VERSION=v0.4.0")
			if err != nil {
				t.Fatalf("release rollback: %v; output: %s", err, out)
			}
			installed, err = os.ReadFile(filepath.Join(dir, "installed"))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(installed), "v0.4.0") {
				t.Error("release rollback did not restore v0.4.0")
			}
		})
	}
}
