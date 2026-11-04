package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const releaseScript = "release.sh"

// releaseScriptPath is resolved while the working directory is still the package
// directory, because these tests run the script inside throwaway repositories.
var releaseScriptPath = func() string {
	path, err := filepath.Abs(releaseScript)
	if err != nil {
		panic(err)
	}

	return path
}()

// runReleaseIn executes the release script in dir with the given environment.
// Stdin is closed, so an unconfirmed run takes the same path a non-interactive
// shell would: read fails, and the release is cancelled rather than guessed.
func runReleaseIn(t *testing.T, dir string, env []string, args ...string) (string, string, int) {
	t.Helper()

	cmd := exec.Command("/bin/sh", append([]string{releaseScriptPath}, args...)...)
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
		t.Fatalf("running %s: %v", releaseScript, err)
	}

	return stdout.String(), stderr.String(), code
}

// gitIn runs git in dir with the hermetic test environment and returns its
// output. It stands in for the manual git commands a releaser would otherwise
// type, and it fails the test when git itself refuses.
func gitIn(t *testing.T, dir string, env []string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}

	return string(out)
}

// commitWithEnv writes a file and commits it with the hermetic identity, unlike
// the shared commit helper which relies on the ambient git configuration.
func commitWithEnv(t *testing.T, dir string, env []string, name string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, env, "add", name)
	gitIn(t, dir, env, "commit", "--quiet", "-m", name)
}

// pushRepo publishes the default branch and every local tag, standing in for the
// merged pull request whose work is about to be released. git refuses to combine
// --all with --tags, so the two refs go up separately.
func pushRepo(t *testing.T, dir string, env []string) {
	t.Helper()

	gitIn(t, dir, env, "push", "--quiet", "origin", "--all")
	gitIn(t, dir, env, "push", "--quiet", "origin", "--tags")
}

// releaseRepoWithTags builds a repository whose release point carries every given
// tag, followed by unreleased commits representing merged work awaiting a
// release. It returns the working tree and the hermetic git environment.
func releaseRepoWithTags(t *testing.T, tags []string, unreleased int) (string, []string) {
	t.Helper()

	work, env := initGitRepo(t)
	commitWithEnv(t, work, env, "one.md")
	for _, name := range tags {
		gitIn(t, work, env, "tag", name)
	}
	for i := 0; i < unreleased; i++ {
		commitWithEnv(t, work, env, "feature-"+strconv.Itoa(i)+".md")
	}
	pushRepo(t, work, env)

	return work, env
}

// releaseRepo is releaseRepoWithTags for the common single-tag case. An empty tag
// stands for a project making its first release.
func releaseRepo(t *testing.T, tag string, unreleased int) (string, []string) {
	t.Helper()

	if tag == "" {
		return releaseRepoWithTags(t, nil, unreleased)
	}

	return releaseRepoWithTags(t, []string{tag}, unreleased)
}

// plan runs a non-mutating release attempt and returns its output and exit code.
func plan(t *testing.T, work string, env []string, args ...string) (string, string, int) {
	t.Helper()

	return runReleaseIn(t, work, append(env, "DRY_RUN=true", "ASSUME_YES=yes"), args...)
}

func TestReleaseBumpLevels(t *testing.T) {
	for _, tt := range []struct {
		level, want string
	}{
		{"", "v0.2.1"},
		{"patch", "v0.2.1"},
		{"minor", "v0.3.0"},
		{"major", "v1.0.0"},
	} {
		t.Run("level="+tt.level, func(t *testing.T) {
			work, env := releaseRepo(t, "v0.2.0", 2)

			args := []string{}
			if tt.level != "" {
				args = append(args, tt.level)
			}
			stdout, stderr, code := plan(t, work, env, args...)
			if code != 0 {
				t.Fatalf("exit code = %d, want 0\nstdout: %s\nstderr: %s", code, stdout, stderr)
			}
			if !strings.Contains(stdout, "release   "+tt.want) {
				t.Errorf("stdout = %q, want it to announce %s", stdout, tt.want)
			}
			if !strings.Contains(stdout, "dry run: git push origin refs/tags/"+tt.want) {
				t.Errorf("stdout = %q, want the planned push of %s", stdout, tt.want)
			}
			if !strings.Contains(stdout, "2 commit(s) since v0.2.0") {
				t.Errorf("stdout = %q, want the released commits listed", stdout)
			}
			if got := strings.TrimSpace(gitIn(t, work, env, "tag", "-l")); got != "v0.2.0" {
				t.Errorf("local tags = %q, want a dry run to create none", got)
			}
			if !strings.Contains(stdout, "Dry run: "+tt.want+" was not tagged") {
				t.Errorf("stdout = %q, want a dry run to say plainly that nothing was published", stdout)
			}
		})
	}
}

func TestReleaseFirstReleaseStartsAtZero(t *testing.T) {
	for _, tt := range []struct {
		level, want string
	}{
		{"patch", "v0.0.1"},
		{"minor", "v0.1.0"},
		{"major", "v1.0.0"},
	} {
		t.Run(tt.level, func(t *testing.T) {
			work, env := releaseRepo(t, "", 1)

			stdout, stderr, code := plan(t, work, env, tt.level)
			if code != 0 {
				t.Fatalf("exit code = %d, want 0\nstdout: %s\nstderr: %s", code, stdout, stderr)
			}
			if !strings.Contains(stdout, "release   "+tt.want+" ("+tt.level+" bump)") {
				t.Errorf("stdout = %q, want %s as a %s bump", stdout, tt.want, tt.level)
			}
			if !strings.Contains(stdout, "first release") {
				t.Errorf("stdout = %q, want it to say this is the first release", stdout)
			}
		})
	}
}

func TestReleaseBaseIgnoresTagsTheWorkflowWouldReject(t *testing.T) {
	// v1.2 and v1.2.3.4 could never publish, and v1.0.0-rc.1 is a retired
	// prerelease. None of them may seed the next stable version, so the bump has to
	// come from the prerelease's stable core.
	work, env := releaseRepoWithTags(t, []string{"v1.2", "v1.2.3.4", "not-a-version", "v1.0.0-rc.1"}, 2)

	stdout, stderr, code := plan(t, work, env, "patch")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "base      v1.0.0-rc.1") {
		t.Errorf("stdout = %q, want the only strict SemVer tag as the base", stdout)
	}
	if !strings.Contains(stdout, "release   v1.0.1 (patch bump)") {
		t.Errorf("stdout = %q, want the prerelease suffix dropped before bumping", stdout)
	}
}

func TestReleaseHonoursRemoteAndDefaultBranch(t *testing.T) {
	work, env := releaseRepo(t, "v1.4.0", 1)
	gitIn(t, work, env, "branch", "-m", "release")
	pushRepo(t, work, env)

	stdout, stderr, code := plan(t, work, append(env, "ORIGIN=origin", "DEFAULT_BRANCH=release"), "minor")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "release   v1.5.0 (minor bump)") {
		t.Errorf("stdout = %q, want v1.5.0", stdout)
	}
}

func TestReleaseUsesExplicitVersion(t *testing.T) {
	work, env := releaseRepo(t, "v0.2.0", 1)

	stdout, stderr, code := runReleaseIn(t, work,
		append(env, "DRY_RUN=true", "ASSUME_YES=yes", "VERSION=v3.1.0"), "patch")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "release   v3.1.0 (explicit VERSION)") {
		t.Errorf("stdout = %q, want the requested version to win over the bump", stdout)
	}
}

func TestReleaseRejections(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(t *testing.T, work string, env []string)
		env    []string
		args   []string
		expect string
	}{
		{
			name: "uncommitted changes",
			setup: func(t *testing.T, work string, env []string) {
				if err := os.WriteFile(filepath.Join(work, "one.md"), []byte("edited"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			expect: "uncommitted changes",
		},
		{
			name: "untracked files do not block a release",
			setup: func(t *testing.T, work string, env []string) {
				if err := os.WriteFile(filepath.Join(work, "scratch-notes.md"), []byte("notes"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			expect: "",
		},
		{
			name: "wrong branch",
			setup: func(t *testing.T, work string, env []string) {
				gitIn(t, work, env, "checkout", "-q", "-b", "feature/unreleased")
			},
			expect: "releases are tagged on main",
		},
		{
			name: "detached head",
			setup: func(t *testing.T, work string, env []string) {
				head := strings.TrimSpace(gitIn(t, work, env, "rev-parse", "HEAD"))
				gitIn(t, work, env, "checkout", "-q", "--detach", head)
			},
			expect: "HEAD is detached",
		},
		{
			name: "commit not pushed",
			setup: func(t *testing.T, work string, env []string) {
				commitWithEnv(t, work, env, "local-only.md")
			},
			expect: "missing from origin/main; merge them through a pull request",
		},
		{
			name: "behind the remote",
			setup: func(t *testing.T, work string, env []string) {
				gitIn(t, work, env, "reset", "--hard", "--quiet", "HEAD~1")
			},
			expect: "missing from HEAD; pull before releasing",
		},
		{
			name: "nothing new since the last release",
			setup: func(t *testing.T, work string, env []string) {
				gitIn(t, work, env, "tag", "v0.2.1")
				pushRepo(t, work, env)
			},
			expect: "nothing new to release",
		},
		{
			name:   "version already tagged",
			env:    []string{"VERSION=v0.2.0"},
			expect: "already exists",
		},
		{
			name:   "version older than the newest release",
			env:    []string{"VERSION=v0.1.0"},
			expect: "does not trail the newest release v0.2.0",
		},
		{
			name:   "version that is not strict SemVer",
			env:    []string{"VERSION=1.2"},
			expect: "not a strict SemVer tag",
		},
		{
			name: "version already published on the remote",
			setup: func(t *testing.T, work string, env []string) {
				// Published tags are discovered even when this checkout has never
				// fetched them, so a second release of the same version cannot be
				// planned from a clone that is behind.
				gitIn(t, work, env, "tag", "v9.9.9")
				pushRepo(t, work, env)
				gitIn(t, work, env, "tag", "-d", "v9.9.9")
			},
			env:    []string{"VERSION=v9.9.9"},
			expect: "already exists",
		},
		{
			name:   "unknown bump level",
			args:   []string{"banana"},
			expect: "unknown bump level banana",
		},
		{
			name:   "invalid dry run flag",
			env:    []string{"DRY_RUN=maybe"},
			expect: "DRY_RUN must be true or false",
		},
		{
			name:   "origin unreachable",
			env:    []string{"ORIGIN=nowhere"},
			expect: "cannot fetch from nowhere",
		},
		{
			name: "missing local identity",
			setup: func(t *testing.T, work string, env []string) {
				// The tagger may not be inferred from the ambient environment, so a
				// repository without a local identity is refused rather than tagged by
				// whoever the machine happens to be logged in as.
				gitIn(t, work, env, "config", "--local", "--remove-section", "user")
			},
			expect: "repository-local git user.name and user.email",
		},
		{
			name:   "absent semver validator is skipped, not fatal",
			env:    []string{"SEMVER=semver-not-installed-here"},
			expect: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			work, env := releaseRepo(t, "v0.2.0", 2)
			if tt.setup != nil {
				tt.setup(t, work, env)
			}

			stdout, stderr, code := runReleaseIn(t, work,
				append(append(env, "DRY_RUN=true", "ASSUME_YES=yes"), tt.env...), tt.args...)

			if tt.expect == "" {
				if code != 0 {
					t.Fatalf("exit code = %d, want 0\nstdout: %s\nstderr: %s", code, stdout, stderr)
				}
				return
			}
			if code == 0 {
				t.Fatalf("exit code = 0, want rejection mentioning %q\nstdout: %s", tt.expect, stdout)
			}
			if !strings.Contains(stderr, tt.expect) {
				t.Errorf("stderr = %q, want it to mention %q", stderr, tt.expect)
			}
			if strings.Contains(stdout, "dry run:") {
				t.Errorf("stdout = %q, want no plan when the release is refused", stdout)
			}
		})
	}
}

func TestReleaseRejectsWhenSemverValidatorDisagrees(t *testing.T) {
	// The bundled validator is a second opinion, never the source of the number.
	// When it rejects what we computed, the release stops rather than publishing a
	// version some other tool will not parse.
	stub := filepath.Join(t.TempDir(), "semver")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\nprintf 'not-a-version\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}

	work, env := releaseRepo(t, "v0.2.0", 1)
	_, stderr, code := runReleaseIn(t, work, append(env, "DRY_RUN=true", "ASSUME_YES=yes", "SEMVER="+stub), "patch")
	if code == 0 {
		t.Fatal("exit code = 0, want the release refused")
	}
	if !strings.Contains(stderr, "does not accept v0.2.1") {
		t.Errorf("stderr = %q, want it to name the rejected version", stderr)
	}
}

func TestReleaseCancelsWithoutConfirmation(t *testing.T) {
	work, env := releaseRepo(t, "v0.2.0", 1)

	stdout, stderr, code := runReleaseIn(t, work, append(env, "DRY_RUN=false"), "patch")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 for a cancelled release\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "release cancelled") {
		t.Errorf("stdout = %q, want it to report the cancellation", stdout)
	}
	if got := strings.TrimSpace(gitIn(t, work, env, "tag", "-l")); got != "v0.2.0" {
		t.Errorf("local tags = %q, want no tag created without a yes", got)
	}
	if out := strings.TrimSpace(gitIn(t, work, env, "ls-remote", "--tags", "origin", "refs/tags/v0.2.1")); out != "" {
		t.Errorf("remote refs = %q, want nothing pushed without a yes", out)
	}
}

func TestReleasePublishesAnnotatedTag(t *testing.T) {
	work, env := releaseRepo(t, "v0.2.0", 2)

	stdout, stderr, code := runReleaseIn(t, work, append(env, "DRY_RUN=false", "ASSUME_YES=yes"), "minor")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "make deploy VERSION=v0.3.0") {
		t.Errorf("stdout = %q, want the follow-up deploy command", stdout)
	}

	if kind := strings.TrimSpace(gitIn(t, work, env, "cat-file", "-t", "v0.3.0")); kind != "tag" {
		t.Errorf("v0.3.0 type = %q, want an annotated tag so the release carries its tagger", kind)
	}
	tagged := strings.TrimSpace(gitIn(t, work, env, "rev-parse", "v0.3.0^{commit}"))
	head := strings.TrimSpace(gitIn(t, work, env, "rev-parse", "HEAD"))
	if tagged != head {
		t.Errorf("v0.3.0 points at %s, want the released commit %s", tagged, head)
	}
	// ls-remote reports the tag object, not the commit it peels to, so compare
	// against the annotated tag's own id.
	tagObject := strings.TrimSpace(gitIn(t, work, env, "rev-parse", "v0.3.0"))
	if out := gitIn(t, work, env, "ls-remote", "--tags", "origin", "refs/tags/v0.3.0"); !strings.Contains(out, tagObject) {
		t.Errorf("origin refs = %q, want the tag published for the workflow to build", out)
	}
}

func TestReleaseUsesTheGivenTagMessage(t *testing.T) {
	work, env := releaseRepo(t, "v1.0.0", 1)

	_, stderr, code := runReleaseIn(t, work,
		append(env, "DRY_RUN=false", "ASSUME_YES=yes", "MESSAGE=Link sanitiser no longer deletes"), "patch")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0\nstderr: %s", code, stderr)
	}

	subject := strings.TrimSpace(gitIn(t, work, env, "for-each-ref", "--format=%(contents:subject)", "refs/tags/v1.0.1"))
	if subject != "Link sanitiser no longer deletes" {
		t.Errorf("tag subject = %q, want the MESSAGE verbatim", subject)
	}
}

func TestReleaseOutsideARepository(t *testing.T) {
	_, env := initGitRepo(t)

	_, stderr, code := runReleaseIn(t, t.TempDir(), append(env, "DRY_RUN=true", "ASSUME_YES=yes"), "patch")
	if code == 0 {
		t.Fatal("exit code = 0, want a refusal to release outside a repository")
	}
	if !strings.Contains(stderr, "not a git repository") {
		t.Errorf("stderr = %q, want it to say the directory is not a repository", stderr)
	}
}

func TestReleaseUsageHelp(t *testing.T) {
	work, env := releaseRepo(t, "v0.2.0", 1)

	stdout, stderr, code := runReleaseIn(t, work, append(env, "DRY_RUN=true"), "--help")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout, "usage:") || !strings.Contains(stdout, "BUMP") {
		t.Errorf("stdout = %q, want usage text on --help", stdout)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want help on stdout only", stderr)
	}
}
