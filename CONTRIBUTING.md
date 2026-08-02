# Contributing

Thanks for taking the time. Small, focused pull requests are easiest to review.

## Setup

You need Go (the version in [`go.mod`](go.mod)) and
[`just`](https://github.com/casey/just). [`goreleaser`](https://goreleaser.com) is
only needed if you touch packaging.

```sh
git clone https://github.com/readeem/hostebin
cd hostebin
just build
just check     # gofmt, go vet (with and without the notsnet tag), go test
```

Try your change end to end:

```sh
just serve &                          # prints a generated token on first run
export HOSTEBIN_SERVER=http://localhost:8080
export HOSTEBIN_TOKEN="$(cat "${XDG_DATA_HOME:-$HOME/.local/share}/hostebin/token")"
just up README.md
```

## Expectations

- `just check` passes. `just test-all` (adds `-tags notsnet` and `-race`) is what CI
  runs on Linux, macOS, and Windows.
- New behaviour comes with a test. The existing tests in `internal/cli`,
  `internal/server`, and `internal/store` are good models.
- Keep the build tag split working: anything touching listeners must compile and
  pass with `-tags notsnet`, which drops embedded Tailscale.
- Match the surrounding style. Comments explain *why*, not *what*.
- Storage and HTTP handling are security-relevant. Do not weaken the path
  containment (`os.OpenRoot`, atomic renames), the bearer-token check, or the
  content-type and CSP handling without saying why in the PR.

## Commit messages

Conventional commits, because the release changelog is generated from them:

```text
feat: add --entry flag to up
fix(store): reject duplicate names in merge mode
docs: clarify Tailscale funnel setup
chore(deps): bump golang.org/x/crypto
```

`feat:` and `fix:` commits are grouped in release notes; `docs:`, `test:`, `chore:`,
and `ci:` are filtered out.

## Versioning and releases

Never hardcode a version. `internal/version` is the single source, overridden by
`-ldflags` in the justfile, Dockerfile, and `.goreleaser.yaml`. Releases are cut by
maintainers with `just tag X.Y.Z`, which pushes a `vX.Y.Z` tag and lets
[`release.yml`](.github/workflows/release.yml) do the rest.

## Reporting bugs

Include the output of `hostebin version`, your OS, and the exact command with the
stderr it produced. For anything security-sensitive, follow
[SECURITY.md](SECURITY.md) instead of opening a public issue.
