# Security notes

## Threat model

`github.com/Trysac/u084-gha-lint` parses and analyses workflow files supplied by the caller. It performs no network access and no process execution.

Defended:

- **Oversized input**: files over 1 MiB are rejected with `ErrTooLarge` before parsing.
- **Alias expansion in YAML**: go-yaml's alias expansion guard applies to parsing.
- **Symlinks during directory scans**: the CLI default scan does not follow symlinks.
- **Untrusted input**: callers may pass hostile strings. The library validates input and returns explicit errors; it does not panic on input.
- **Supply chain**: the only dependency outside the standard library is `go.yaml.in/yaml/v3` (v3.0.4). Mitigations: `go.sum` verification, `govulncheck` in the local gates, Dependabot weekly updates, actions pinned by commit SHA, minimal workflow permissions.
- **Leaked secrets**: `gitleaks` runs in the local gates and in CI; `.env` files are git-ignored.

Not defended:

- **Analysis limits**: the lint rules are static and heuristic. GHA005 matches a fixed list of 22 untrusted contexts and does not perform expression typing, so other untrusted inputs are not reported.
- **Cost estimate**: the minutes figure is an upper-bound estimate, not a bill. It uses the bundled or configured multipliers and does not model runner queueing or cache effects.
- **Trust in configuration**: multipliers and `allowTagOwners` come from the config file and are trusted as provided. Owners listed in `allowTagOwners` are exempt from GHA003.
- **Composite actions**: they are skipped and not analysed.

## Publishing

Releases are git tags served through the Go module proxy and checksum database. No registry token is stored in the repository.

To report a vulnerability, see [SECURITY.md](../SECURITY.md).
