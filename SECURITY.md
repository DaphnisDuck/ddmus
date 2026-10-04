# Security

## Reporting a vulnerability

Please report it privately, not in a public issue: open a report under **Security → Report a vulnerability** on [the ddmus repository](https://github.com/DaphnisDuck/ddmus/security/advisories/new).

If that page isn't available to you, write to daphnisduck@wompernet.us.

Say what you found, the ddmus version (`ddmus --version`), and how to reproduce it. ddmus is maintained by one person in spare time: expect an answer within a week, and a fix or a plan for one within 30 days for anything serious. You'll be credited in the release notes unless you'd rather not be.

## What is supported

Until 1.0 is released: the `main` branch. After that: the latest 1.x release. Fixes are not ported to older versions.

## Official builds

No binaries are published yet; ddmus is built from source (see the [README](README.md#install)).

From 1.0, the official binaries will be the files on this repository's Releases page and the AUR package `ddmus-bin`, which installs those files. Each release will carry `SHA256SUMS` and a build attestation, to check a download with:

```sh
sha256sum -c --ignore-missing SHA256SUMS
gh attestation verify ddmus-<version>-linux-amd64.tar.gz --repo DaphnisDuck/ddmus
```

A vulnerability in cliamp, which ddmus is forked from, should also go to [cliamp](https://github.com/bjarneo/cliamp).

## If ddmus stops being maintained

The repository will be archived with a notice saying so, and `ddmus-bin`, once it exists, will be handed to a named co-maintainer or deleted from the AUR on request. It won't be left orphaned for someone else to adopt silently.
