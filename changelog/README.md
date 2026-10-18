# Release notes

One Markdown file per release, named after its version: `1.2.0.md`. The
release workflow (`.github/workflows/release.yml`) refuses a tag without
one. The same text becomes the GitHub release body and the launcher's
"What changed" dialog, which shows each line on its own: headings, list
markers, bold and code marks are dropped, and blank lines are skipped.
Write for players, not for developers.

## Releasing

```sh
$EDITOR changelog/1.2.0.md && git add changelog/1.2.0.md && git commit -m "Release notes for 1.2.0"
git tag v1.2.0 && git push origin main v1.2.0
```

Then open the run in the Actions tab. Once the checks and the builds have
passed, **Review deployments → release → Approve** signs `update.json` and
publishes the release. Installed launchers pick it up within 12 hours, or
right away with Settings → Updates → Check now.

`go run ./tools/release` (build, manifest, sign) still works by hand, as a
fallback.

## One-time setup

1. `go run ./tools/release keygen`. Back up `~/.config/quartz-release/quartz.key`
   and its passphrase somewhere offline: every installed launcher trusts
   this key, and without it they can't be updated again.
2. Repository → Settings → Secrets and variables → Actions:
   - variable `QUARTZ_RELEASE_PUBKEY`: the public key line keygen printed
   - secrets `QUARTZ_MSA_CLIENT_ID`, `QUARTZ_CF_API_KEY`, `QUARTZ_DISCORD_CLIENT_ID`
3. Settings → Environments → new environment `release`:
   - required reviewer: yourself
   - deployment branches and tags: selected, tag rule `v*`
   - environment secrets `QUARTZ_RELEASE_KEY` (the whole content of
     `quartz.key`) and `QUARTZ_RELEASE_PASSPHRASE`
4. Settings → Rules → new tag ruleset for `v*`: restrict creation, update and
   deletion to yourself.

The build jobs only get the public key. The secret key exists only in the
publish job, after you approve, and is deleted when that job ends. Signing
fails if it doesn't belong to `QUARTZ_RELEASE_PUBKEY`.
