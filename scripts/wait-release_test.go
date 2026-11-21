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

func TestWaitRelease(t *testing.T) {
	for _, tt := range []struct {
		scenario  string
		wantError bool
		want      string
		wantWatch bool
	}{
		{"published", false, "", false},
		{"running", false, "Watching https://github.com/example/project/actions/runs/42", true},
		{"delayed-run", false, "Watching", true},
		{"delayed-assets", false, "Watching", true},
		{"failed", true, "Release publication failed", true},
		{"cancelled", true, "Release publication failed", true},
		{"missing-run", true, "No Publish release run found", false},
		{"missing-assets", true, "release assets are unavailable", true},
		{"api-error", true, "authentication and connectivity", false},
	} {
		t.Run(tt.scenario, func(t *testing.T) {
			dir := t.TempDir()
			stub := `#!/bin/sh
set -eu
printf '%s\n' "$*" >> "$TEST_STATE/calls"
case "$1 $2" in
'release view')
 [ "$3" = v1.2.3 ] && [ "$4" = --repo ] && [ "$5" = example/project ] || exit 99
 if [ "$SCENARIO" = published ]; then echo true
 elif [ -f "$TEST_STATE/watched" ]; then
  case "$SCENARIO" in
   missing-assets) echo false ;;
   delayed-assets)
    if [ -f "$TEST_STATE/assets-checked" ]; then echo true
    else touch "$TEST_STATE/assets-checked"; echo false; fi ;;
   *) echo true ;;
  esac
 else echo false; fi
 ;;
'run list')
 [ "$3" = --repo ] && [ "$4" = example/project ] &&
 [ "$5" = --workflow ] && [ "$6" = publish.yml ] &&
 [ "$7" = --branch ] && [ "$8" = v1.2.3 ] &&
 [ "$9" = --event ] && [ "${10}" = push ] || exit 99
 case "$SCENARIO" in
  missing-run) ;;
  api-error) exit 1 ;;
  delayed-run)
   if [ -f "$TEST_STATE/listed" ]; then echo 42
   else touch "$TEST_STATE/listed"; fi ;;
  *) echo 42 ;;
 esac
 ;;
'run watch')
 [ "$3" = 42 ] && [ "$4" = --repo ] && [ "$5" = example/project ] &&
 [ "$6" = --exit-status ] || exit 99
 case "$SCENARIO" in failed|cancelled) exit 1 ;; esac
 touch "$TEST_STATE/watched"
 ;;
*) exit 99 ;;
esac
`
			for name, body := range map[string]string{"gh": stub, "sleep": "#!/bin/sh\nexit 0\n"} {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0700); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "sh", "wait-release.sh", "example/project", "v1.2.3")
			cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"), "TEST_STATE="+dir, "SCENARIO="+tt.scenario)
			out, err := cmd.CombinedOutput()
			if (err != nil) != tt.wantError {
				t.Fatalf("error = %v, output: %s", err, out)
			}
			if !strings.Contains(string(out), tt.want) {
				t.Errorf("output = %s, want %q", out, tt.want)
			}
			calls, err := os.ReadFile(filepath.Join(dir, "calls"))
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Contains(string(calls), "run watch"); got != tt.wantWatch {
				t.Errorf("watched = %v, want %v; calls: %s", got, tt.wantWatch, calls)
			}
			if tt.scenario == "published" && strings.Contains(string(calls), "run list") {
				t.Error("published release should skip workflow discovery")
			}
		})
	}
}
