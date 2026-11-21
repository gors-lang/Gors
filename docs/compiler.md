# Compiler and bootstrap guide

GoRs ships a main compiler written in Go and a bootstrap compiler written in
GoRs. The bootstrap is used to grow the set of programs GoRs can compile
without a Go toolchain. The two compilers currently support different subsets.
For user-facing capabilities, see [Two compilers](../README.md#two-compilers).
Paths to implementation files below refer to the Git checkout.

## Main compiler

1. [`cmd/gors`](../cmd/gors/) handles `emit`, `build`, and `run`, including backend
   selection, output paths, and program arguments.
2. [`internal/lexer`](../internal/lexer/) tokenizes source. The
   [parser](../internal/parser/) uses those tokens to build the
   [AST](../internal/ast/).
3. [`CompileFile`](../internal/compiler/modules.go) loads and resolves local imports
   and links declarations. `CompileWith` in
   [`compiler.go`](../internal/compiler/compiler.go) handles an in-memory program
   with no imports.
4. [`internal/sema`](../internal/sema/) type checks and then checks moves and
   borrows before code generation.
5. [`internal/cgen`](../internal/cgen/) emits C23 and
   [`internal/codegen`](../internal/codegen/) emits Go. The CLI invokes the
   appropriate toolchain for `build` and `run`.

## Bootstrap compiler

The entry point is [`bootstrap/cgen/main.gors`](../bootstrap/cgen/main.gors).
The bootstrap has its own [lexer](../bootstrap/lexer/),
[parser and import handling](../bootstrap/parser/),
[semantic checker](../bootstrap/sema/), and
[C emitter and runtime support](../bootstrap/cgen/).
Its native CLI drives GCC for `build` and `run`.

When changing the bootstrap, check that it can still compile itself. The
self-host test builds a seed using the main compiler, uses that seed to emit C
from the bootstrap sources, then compiles the emitted C with GCC. The resulting
executable emits C from the same sources again. The two outputs must match byte
for byte. This checks reproducibility across consecutive stages.

```sh
go test ./internal/compiler -run '^TestBootstrapSelfHost$' -count=1
```

The README shows how to [run those stages manually](../README.md#reproduce-the-self-build).

## Check release archives

From the Git checkout, build the release archives after changes to bootstrapping
or packaging. The packaging tool verifies the self-build and writes Linux and
Windows amd64 archives with `SHA256SUMS`.

```sh
go run ./tools/package -out dist
```

On Linux, pass the resulting Linux archive to the archive smoke test. If
`dist` contains multiple versions, pass the exact archive path instead of the
glob.

```sh
bash tools/check-linux-archive.sh dist/gors-*-linux-amd64.tar.gz
```

The script builds the bundled compiler with only GCC and its assembler and
linker on PATH, compares regenerated C with the bundled C, and exercises native
`build` and `run`. The [CI workflow](../.github/workflows/ci.yml) also runs these
checks.
