# Contributing to Alpage

Alpage is licensed under the Business Source License 1.1 (see [LICENSE](LICENSE)). Copyright is held by Swiss Subnet AG.

## Sign-off (DCO)

Contributions are accepted under the Developer Certificate of Origin 1.1. Sign off each commit:

```
git commit -s
```

That appends a trailer with the name and email from your git config:

```
Signed-off-by: Your Name <you@example.com>
```

By signing off you certify the statement reproduced below. Commits without a sign-off are rejected by CI. To fix a branch you already wrote, `git rebase --signoff main`.

If you contribute on behalf of an employer, make sure you have their permission: your sign-off asserts you have the right to submit the work.

## Before opening a pull request

- `go test ./...` passes.
- `go generate ./nns` is re-run if you changed struct tags, so [docs/config.md](docs/config.md) stays in sync.
- New proposal kinds are dry-run verified against PocketIC.

## Developer Certificate of Origin 1.1

The full text, reproduced verbatim from <https://developercertificate.org/>:

```
Developer Certificate of Origin
Version 1.1

Copyright (C) 2004, 2006 The Linux Foundation and its contributors.

Everyone is permitted to copy and distribute verbatim copies of this
license document, but changing it is not allowed.


Developer's Certificate of Origin 1.1

By making a contribution to this project, I certify that:

(a) The contribution was created in whole or in part by me and I
    have the right to submit it under the open source license
    indicated in the file; or

(b) The contribution is based upon previous work that, to the best
    of my knowledge, is covered under an appropriate open source
    license and I have the right under that license to submit that
    work with modifications, whether created in whole or in part
    by me, under the same open source license (unless I am
    permitted to submit under a different license), as indicated
    in the file; or

(c) The contribution was provided directly to me by some other
    person who certified (a), (b) or (c) and I have not modified
    it.

(d) I understand and agree that this project and the contribution
    are public and that a record of the contribution (including all
    personal information I submit with it, including my sign-off) is
    maintained indefinitely and may be redistributed consistent with
    this project or the open source license(s) involved.
```
