# Contributing to GoRs

Changes to the compiler, examples, tests, and documentation are welcome. Start
with the [documentation index](docs/README.md) or the [compiler guide](docs/compiler.md)
if you're looking for the right part of the codebase.

## Set up a local checkout

Install Go 1.22+ and GCC 14+ with C23 support. On Windows, CI uses MSYS2 UCRT64
GCC with `C:\msys64\ucrt64\bin` on PATH. Run these commands from the repository root
in Bash or PowerShell.

```sh
go version
gcc --version
go run ./cmd/gors --version
go run ./cmd/gors run examples/hello.gors --backend c
go run ./cmd/gors run examples/hello.gors --backend go
```

## Test your change

Run the full suite before opening a pull request. On Windows, CI runs the same
suite with `-parallel 2` to limit concurrent test processes.

```sh
go test -count=1 ./...
go vet ./...
```

For a shorter feedback loop, run the tests closest to the change.

| Change | Where to add coverage | Focused command |
| --- | --- | --- |
| Types, borrowing, or module diagnostics | [`compiler_test.go`](internal/compiler/compiler_test.go), [`v01_test.go`](internal/compiler/v01_test.go) | `go test ./internal/compiler -run 'TestRejects\|TestV01\|TestModuleErrors' -count=1` |
| Program behavior across backends | [`execution_test.go`](internal/compiler/execution_test.go) | `go test ./internal/compiler -run TestBackendExecution -count=1` |
| Bootstrap frontend | [`bootstrap_lexer_test.go`](internal/compiler/bootstrap_lexer_test.go) | `go test ./internal/compiler -run 'TestBootstrapLexer\|TestBootstrap.*Parser\|TestBootstrapSemanticChecker' -count=1` |
| Bootstrap emission, native CLI, or self-hosting | [`bootstrap_cgen_test.go`](internal/compiler/bootstrap_cgen_test.go) | `go test ./internal/compiler -run 'TestBootstrapCEmitter\|TestBootstrapNativeCLI\|TestBootstrapSelfHost' -count=1` |

For an output or exit-status bug, add a small GoRs program and assert what
the process prints or returns. For a diagnostic bug, include the shortest
rejected program that reproduces it. Format changed Go files with `gofmt`.
For changes to language behavior, update the affected examples and
[language reference](docs/language-v0.1.md).

## Report a bug or open a pull request

When reporting a compiler bug, include a short `.gors` reproducer, the command
you ran, expected and actual results, and your OS and compiler version. Identify
whether you used `gors` or `gors-bootstrap`. For the main compiler, include the
backend, and for C builds include the output of `gcc --version`.
[Open an issue](https://github.com/gors-lang/Gors/issues/new)

In a pull request, describe the behavior that changed, include the commands you
used to verify it, and link the issue if one exists. If the change affects the
bootstrap subset, mention what you verified on both compilers.
