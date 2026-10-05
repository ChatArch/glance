# Maintained Go binary contract

This repository (`ChatArch/glance`) owns the maintained **Go Glance web binary** and its `chatarch-vMAJOR.MINOR.PATCH` release tags. `glanceapp/glance` is upstream; its `v*` tags, GoReleaser archives and Docker images are **not** ChatArch binaries. The separately maintained ChatGlance Python package owns collection, rendering and orchestration of Glance configuration and data, not the `glance` executable or its server process. Installing the Python package does not replace the Go binary. Use a matching maintained fork tag and source revision; do not infer optional-login support from an upstream binary or a Python package version.

## Release files and verification

The maintained tag workflow builds only **Linux amd64**, with `CGO_ENABLED=0`. For a tag such as `chatarch-v0.1.0`, the exact archive name is `glance-chatarch-v0.1.0-linux-amd64.tar.gz`; it contains an executable named `glance`. There are no maintained fork Windows, macOS, ARM, or Docker release artifacts in this workflow. The binary prints `chatarch-v0.1.0+<full source commit SHA>` using `glance --version` (or `-v` or `version`). The tag must resolve to that same commit; compare both to the chosen release's source, rather than pairing unrelated binaries and config revisions.

Future releases made **after this workflow change is merged into the tagged source** upload `BUILDINFO.txt` (tag, full source SHA, binary version, archive name, platform and CGO setting) and `SHA256SUMS` alongside the archive. `SHA256SUMS` covers the downloadable archive and `BUILDINFO.txt` (not itself); check with `sha256sum -c SHA256SUMS` in the directory containing all three files. Check the SHA, tag and binary version in `BUILDINFO.txt` against the source tag and `glance --version`. The already published `chatarch-v0.1.0` release may predate `BUILDINFO.txt`; its existing workflow produced an archive and `SHA256SUMS` for the archive only. Do not assume old releases have the new manifest, or infer remotely available assets from this source tree alone.

## Portable configuration

Glance reads YAML via `glance --config /path/to/glance.yml`. Its existing interpolation supports `${NAME}` or `${env:NAME}` for environment variables named with uppercase letters, digits and underscores; a missing variable fails loading. `${readFileFromEnv:NAME}` reads and trims an **absolute** file path provided in that environment variable; `${secret:name}` reads and trims `/run/secrets/name`. `\${NAME}` escapes interpolation. Keep the YAML template free of credential values. ChatGlance/ChatEnv should inject environment variables or arrange restricted secret files in the Glance process environment at launch; this repo does not ship or store credentials in generic YAML.

The existing optional-login fields are top-level `auth.users`, `auth.secret-key`, page `public: true`, and `authenticated-columns` on a public page. With users configured, pages default to private. The authenticated columns replace public columns only for a verified session; shared head/widgets and assets must be safe for guests. For example, use placeholders (not credentials):

```yaml
auth:
  secret-key: ${GLANCE_AUTH_SECRET_KEY}
  users:
    admin:
      password-hash: ${GLANCE_ADMIN_PASSWORD_HASH}
pages:
  - name: Projects
    public: true
    columns:
      - size: full
        widgets:
          - type: html
            source: Public projects only
    authenticated-columns:
      - size: full
        widgets:
          - type: html
            source: Signed-in projects only
```

Generate a key with `glance secret:make` and a hash with the documented password-hash CLI; inject their **values** at runtime rather than recording them here. For optional login, review [public pages and optional login](configuration.md#public-pages-and-optional-login). Do not treat generated public/private candidate files as shipped Go features or proof of a private component's release.
