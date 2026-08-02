# Security policy

## Reporting a vulnerability

Please report privately through GitHub's
[private vulnerability reporting](https://github.com/readeem/hostebin/security/advisories/new)
rather than in a public issue. Include a description, affected version
(`hostebin version`), and reproduction steps.

Expect an acknowledgement within a week. Fixes ship in a patch release with credit in
the release notes unless you prefer otherwise.

## Supported versions

The latest release. There are no long-term support branches.

## Design notes for reporters

These are known, documented properties rather than vulnerabilities:

- **Bundle IDs are the read capability.** Content under `/b/{id}/` is public to
  anyone holding the 128-bit ID; there is no per-bundle authentication.
- **Bundles share one origin.** The default CSP allows inline scripts and HTTPS CDN
  assets so generated reports render. Content in one bundle can therefore reach
  another bundle whose ID it already knows. Per-bundle origins are out of scope for
  v1 and tracked as a design limitation, not a bug.
- **All mutations require the bearer token.** Anything that creates, replaces, or
  deletes a bundle without a valid `Authorization: Bearer` header *is* a bug.

Things worth reporting: escaping the storage root (`..`, symlinks, path
normalization), bypassing the token check, resource exhaustion beyond the configured
`--max-upload` / `--max-files` limits, content served with an executable or
unexpectedly permissive content type, CSP or `nosniff` headers missing from bundle
responses, and anything that leaks the server token or another bundle's ID.
