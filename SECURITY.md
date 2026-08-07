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
- **Reads stay public and unauthenticated.** The 128-bit bundle ID is still the read
  capability. User isolation stops one *regular* user listing, replacing, or deleting
  another user's bundles; admins reach every bundle by design. Isolation does not make
  bundle URLs private or change the shared-origin caveat above.
- **All mutations require a live bearer token.** Each user has at most one token,
  stored as a SHA-256 digest. Rotating it invalidates the previous token immediately,
  and disabled users cannot authenticate. Anything that
  creates, replaces, or deletes a bundle without a valid `Authorization: Bearer`
  header *is* a bug.

If a token leaks, revoke it immediately with `hostebin token rm`; an admin uses
`hostebin token rm --user <name>`. Create a replacement with `hostebin token new`
or `hostebin token new --user <name>`. Revocation is immediate:
the next request using that token must receive `401`.

Things worth reporting: escaping the storage root (`..`, symlinks, path
normalization), bypassing the token check, resource exhaustion beyond the configured
`--max-upload` / `--max-files` limits, content served with an executable or
unexpectedly permissive content type, CSP or `nosniff` headers missing from bundle
responses, and anything that leaks the server token or another bundle's ID.
