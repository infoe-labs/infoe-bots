# Agent privacy and commit policy

This is a public, pseudonymous repository. Treat every tracked file, commit,
tag, pull request, release artifact, and CI log as permanently public.

## Identity safety

- Use only the repository-local Git author and committer identity already
  configured and approved by the local identity hooks.
- Never infer an identity from the operating-system user, hostname, global Git
  configuration, GitHub login, shell prompt, home directory, or another
  repository.
- Never replace the configured identity or add a new identity without explicit
  user approval.
- Never bypass commit or push hooks with `--no-verify` or an equivalent option.
- GitHub-created commits must use the project's public alias and GitHub's
  private `noreply` address. Audit their metadata immediately after merging.

## Information that must remain local

Do not commit, paste into PR text, emit into CI logs, or publish in release
metadata any personal name, personal email, account name, system username,
hostname, IP address, physical location, absolute local path, shell prompt,
secret, token, local environment file, or local hook implementation.

Local hooks belong under `.git/hooks`; they must never be copied into the
repository. Secrets and machine-specific deployment configuration must remain
in ignored local files.

## Required checks before publishing

Before every commit and push:

1. Inspect the staged diff for identity data, secrets, local paths, and machine
   details.
2. Confirm the pending commit's author and committer match the approved
   repository-local identity.
3. Run the local privacy hooks normally and stop if they reject the operation.
4. Inspect every commit in the outgoing range, including metadata and tracked
   contents, before pushing.

If a privacy check fails, do not weaken or bypass it. Fix the offending content
or identity. A local hook may be narrowly updated only when it rejects an
already-approved public GitHub `noreply` merge identity; such updates must stay
local and must not permit personal identities.
