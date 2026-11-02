package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const script = "deploy-drift.sh"

// scriptPath is resolved once while the working directory is still the package
// directory, because repository tests run the script from inside a throwaway
// repository where a relative path would not resolve.
var scriptPath = func() string {
	path, err := filepath.Abs(script)
	if err != nil {
		panic(err)
	}

	return path
}()

// runScript executes the deployment drift check with the given environment and
// returns stdout, stderr, and the exit code. It never calls the real network:
// tests inject either a stub git or a local on-disk remote.
func runScript(t *testing.T, env []string, args ...string) (string, string, int) {
	return runScriptIn(t, "", env, args...)
}

// runScriptIn is runScript with an explicit working directory. Repository-based
// tests must pin it, otherwise the script would resolve origin against the real
// checkout instead of the throwaway repository under test.
func runScriptIn(t *testing.T, dir string, env []string, args ...string) (string, string, int) {
	t.Helper()

	cmd := exec.Command("/bin/sh", append([]string{scriptPath}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	code := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		code = exitErr.ExitCode()
	} else if err != nil {
		t.Fatalf("running %s: %v", script, err)
	}

	return stdout.String(), stderr.String(), code
}

// fakeGit installs a stub git that answers `rev-list --count` with count and
// succeeds at everything else. An empty count makes it report nothing.
func fakeGit(t *testing.T, count string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "git")
	body := "#!/bin/sh\n"
	if count != "" {
		body += "if [ \"$1\" = rev-list ]; then printf '" + count + "'; fi\nexit 0\n"
	} else {
		body += "if [ \"$1\" = rev-list ]; then exit 1; fi\nexit 0\n"
	}
	if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}

	return path
}

func TestDriftMessageWarnsWhenBehind(t *testing.T) {
	stub := fakeGit(t, "3")

	stdout, stderr, code := runScript(t, []string{"GIT=" + stub}, "v0.1.1")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (the check must never block a deploy)", code)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty so progress output stays clean", stdout)
	}
	want := "warning: release v0.1.1 is 3 commits behind origin/main\n"
	if stderr != want {
		t.Errorf("stderr = %q, want %q", stderr, want)
	}
}

func TestDriftMessageSingularAndPlural(t *testing.T) {
	for _, tt := range []struct {
		count, wantFragment string
	}{
		{"1", "is 1 commit behind"},
		{"2", "is 2 commits behind"},
		{"10", "is 10 commits behind"},
	} {
		t.Run(tt.count, func(t *testing.T) {
			_, stderr, _ := runScript(t, []string{"GIT=" + fakeGit(t, tt.count)}, "v1.0.0")
			if !strings.Contains(stderr, tt.wantFragment) {
				t.Errorf("stderr = %q, want it to contain %q", stderr, tt.wantFragment)
			}
		})
	}
}

func TestDriftMessageSilentWhenNotBehind(t *testing.T) {
	// Zero, empty, and non-numeric counts must all stay silent. A count we cannot
	// trust may never invent a warning that sends someone chasing a phantom.
	for _, count := range []string{"0", "", "abc", "0.5", "-2"} {
		t.Run("count="+count, func(t *testing.T) {
			stdout, stderr, code := runScript(t, []string{"GIT=" + fakeGit(t, count)}, "v0.1.1")
			if stdout != "" || stderr != "" {
				t.Errorf("output = (%q, %q), want silent", stdout, stderr)
			}
			if code != 0 {
				t.Errorf("exit code = %d, want 0", code)
			}
		})
	}
}

func TestDriftMessageHonoursRemoteAndBranch(t *testing.T) {
	stub := fakeGit(t, "4")

	_, stderr, _ := runScript(t, []string{"GIT=" + stub, "ORIGIN=upstream", "DEFAULT_BRANCH=release"}, "v2.0.0")
	want := "warning: release v2.0.0 is 4 commits behind upstream/release\n"
	if stderr != want {
		t.Errorf("stderr = %q, want %q", stderr, want)
	}
}

func TestDriftSilentWhenGitCannotAnswer(t *testing.T) {
	// A missing binary stands in for offline or non-repo environments: the
	// deployment must proceed and stay quiet rather than fail or guess.
	_, stderr, code := runScript(t, []string{"GIT=" + filepath.Join(t.TempDir(), "no-such-git")}, "v0.1.1")
	if stderr != "" {
		t.Errorf("stderr = %q, want silent", stderr)
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}

func TestDriftUsageOnBadArgs(t *testing.T) {
	_, stderr, code := runScript(t, nil)
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if !strings.Contains(stderr, "usage:") {
		t.Errorf("stderr = %q, want usage text", stderr)
	}
}

// initGitRepo builds a real repository backed by an on-disk bare "origin", so
// the fetch path is exercised without any network access. It returns the working
// tree and the environment that keeps git hermetic and pointed at it.
func initGitRepo(t *testing.T) (string, []string) {
	t.Helper()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}

	root := t.TempDir()
	remote := filepath.Join(root, "origin.git")
	work := filepath.Join(root, "work")

	env := []string{
		"GIT_CONFIG_NOSYSTEM=1",
		"HOME=" + root,
		"GIT_AUTHOR_NAME=test",
		"GIT_AUTHOR_EMAIL=test.invalid",
		"GIT_COMMITTER_NAME=test",
		"GIT_COMMITTER_EMAIL=test.invalid",
	}

	run := func(dir, name string, args ...string) {
		t.Helper()
		cmd := exec.Command(name, args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), env...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	if err := os.MkdirAll(remote, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}

	run(remote, "git", "init", "--quiet", "--bare", "--initial-branch=main", ".")
	run(work, "git", "clone", "--quiet", remote, ".")
	run(work, "git", "config", "user.email", "test.invalid")
	run(work, "git", "config", "user.name", "test")

	return work, env
}

func commit(t *testing.T, dir, name string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "add", name)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}
	cmd = exec.Command("git", "commit", "--quiet", "-m", name)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}
}

func TestDriftAgainstRealRepository(t *testing.T) {
	work, env := initGitRepo(t)

	commit(t, work, "one.md")
	commit(t, work, "two.md")
	cmd := exec.Command("git", "tag", "v0.1.0")
	cmd.Dir = work
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git tag: %v\n%s", err, out)
	}

	// Two further commits represent the merged-but-unreleased feature work.
	commit(t, work, "three.md")
	commit(t, work, "four.md")

	cmd = exec.Command("git", "push", "--quiet", "origin", "main", "refs/tags/v0.1.0")
	cmd.Dir = work
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git push: %v\n%s", err, out)
	}

	t.Run("up to date release is silent", func(t *testing.T) {
		commit(t, work, "five.md")
		cmd := exec.Command("git", "tag", "v0.2.0")
		cmd.Dir = work
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git tag: %v\n%s", err, out)
		}
		cmd = exec.Command("git", "push", "--quiet", "origin", "refs/tags/v0.2.0")
		cmd.Dir = work
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git push tag: %v\n%s", err, out)
		}

		_, stderr, code := runScriptIn(t, work, env, "v0.2.0")
		if stderr != "" {
			t.Errorf("stderr = %q, want silent for a current release", stderr)
		}
		if code != 0 {
			t.Errorf("exit code = %d, want 0", code)
		}
	})

	t.Run("stale release warns", func(t *testing.T) {
		_, stderr, code := runScriptIn(t, work, env, "v0.1.0")
		want := "warning: release v0.1.0 is 2 commits behind origin/main\n"
		if stderr != want {
			t.Errorf("stderr = %q, want %q", stderr, want)
		}
		if code != 0 {
			t.Errorf("exit code = %d, want 0", code)
		}
	})

	// `make deploy` discovers tags with `git ls-remote`, which never creates local
	// objects. Forgetting to fetch the tag would silently skip the very warning
	// this script exists to print, so drop the local tag and retry.
	t.Run("warns for a tag that is not fetched locally", func(t *testing.T) {
		cmd := exec.Command("git", "tag", "-d", "v0.1.0")
		cmd.Dir = work
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git tag -d: %v\n%s", err, out)
		}

		_, stderr, code := runScriptIn(t, work, env, "v0.1.0")
		want := "warning: release v0.1.0 is 2 commits behind origin/main\n"
		if stderr != want {
			t.Errorf("stderr = %q, want %q", stderr, want)
		}
		if code != 0 {
			t.Errorf("exit code = %d, want 0", code)
		}
	})

	t.Run("unknown version stays silent", func(t *testing.T) {
		_, stderr, code := runScriptIn(t, work, env, "v9.9.9")
		if stderr != "" {
			t.Errorf("stderr = %q, want silent for an unfetchable version", stderr)
		}
		if code != 0 {
			t.Errorf("exit code = %d, want 0", code)
		}
	})
}
