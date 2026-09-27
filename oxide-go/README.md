# oxide-go

Go backend and runtime for Oxide. See the [project README](../README.md) for
architecture, build commands, storage contracts and the renderer reproduction.
[The support ledger](../MILESTONES.md) distinguishes tested subsets from
unimplemented APIs.

Translation uses pinned rustc MIR and actual target layouts. The rust-analyzer
parser is used only by `oxide audit` for syntax inventory. Generated packages
import `github.com/csbxd/oxide/oxide-go/runtime` and are specific to Linux amd64
or arm64.

Rust heap storage uses `modernc.org/memory`; C/OS boundaries use
`modernc.org/libc`. Zero Go allocations in warmed differential tests does not
mean zero Rust heap allocations. Addressable automatic storage uses reusable
Context frames outside the Go stack; native physical stack equivalence is not
claimed.

```sh
go test ./...
OXIDE_RUST_TESTS=1 go test ./internal/mir -run '^TestRust' -timeout 30m -v
```
