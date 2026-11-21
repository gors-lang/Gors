# GoRs

[![CI](https://github.com/rxzsu/Gors/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/rxzsu/Gors/actions/workflows/ci.yml)
[![Prerelease v1.0.0-alpha.2](https://img.shields.io/badge/release-v1.0.0--alpha.2-df9f1d)](https://github.com/rxzsu/Gors/releases/tag/v1.0.0-alpha.2)

GoRs is an experimental compiled language that pairs a small, Go-inspired syntax with Rust-inspired ownership and borrowing. It targets C23 and Go. The bootstrap compiler, written in GoRs, can now compile its own sources through C23 and reproduce the same generated C in the next stage.

[Quick start](#quick-start) · [Language reference](docs/language-v0.1.md) · [Examples](examples/) · [Changelog](CHANGELOG.md) · [Русский](README.ru.md)

## Quick start

Download and extract the archive for your platform. To compile programs with the C backend, put a C23 compiler such as GCC 14+ on your PATH.

| Platform | Download | Run from the extracted directory |
| --- | --- | --- |
| Windows amd64 | [ZIP](https://github.com/rxzsu/Gors/releases/download/v1.0.0-alpha.2/gors-1.0.0-alpha.2-windows-amd64.zip) | `.\gors.exe run examples/hello.gors --backend c` |
| Linux amd64 | [TAR.GZ](https://github.com/rxzsu/Gors/releases/download/v1.0.0-alpha.2/gors-1.0.0-alpha.2-linux-amd64.tar.gz) | `./gors run examples/hello.gors --backend c` |

The program prints `hello from GoRs`. [SHA-256 checksums](https://github.com/rxzsu/Gors/releases/download/v1.0.0-alpha.2/SHA256SUMS)

## What the language looks like

This function borrows a point to compute the square of its distance from the origin. The caller can still use the point after the call.

```rust
struct Point {
    x: int,
    y: int,
}

fn (p: &Point) norm2() -> int {
    return p.x * p.x + p.y * p.y
}

fn main() {
    let p = Point { x: 3, y: 4 }
    println(p.norm2()) // 25
    println(p.x)       // 3
}
```

Bindings are immutable unless declared with `let mut`. Strings, slices, and structures move when passed by value. Read through `&T` or borrow exclusively through `&mut T`. The compiler checks moves and conflicting borrows before generating code. [Ownership rules](docs/language-v0.1.md#ownership-and-references)

`Result` makes failure part of the return type. Here `?` forwards a read error, while `match` handles both outcomes at the call site.

```rust
fn file_size(path: string) -> Result<int, string> {
    let bytes = read_bytes(path)?
    return Ok(len(bytes))
}

fn main() {
    match file_size("README.md") {
        Ok(size) => println(size, "bytes"),
        Err(message) => eprintln(message),
    }
}
```

| Example | What it shows |
| --- | --- |
| [Methods](examples/methods.gors) | Shared and mutable receivers |
| [Point by value](examples/point.gors) | Struct literals and field reads in the bootstrap compiler |
| [Modules](examples/modules/main.gors) | File imports and public declarations |
| [Option](examples/option.gors) | Values that can be present (`Some`) or absent (`None`) |
| [Result](examples/result.gors) | Error handling and early return with `?` |
| [Slices](examples/slices.gors) | Typed arrays of elements, indexing, and mutation |
| [File counter](examples/wc.gors) | Command-line arguments, binary I/O, and exhaustive `match` |

## Two compilers

| Compiler | Current scope |
| --- | --- |
| `gors`, written in Go | C23 and Go backends, file modules, structs and methods, `Option`, `Result`, and ownership checks |
| `gors-bootstrap`, written in GoRs | A self-hosting lexer, parser, type checker, and C23 emitter. Its subset includes file modules, structs, methods, slices, references, `Option`, `Result`, and `?` |

The bootstrap compiler can build its own sources and the bundled examples listed below with GCC. Import paths are normalized lexically. The bootstrap supports shared local references, mutable reference arguments, and by-value struct and slice payloads in `Option` and `Result`. Returned references, local mutable reference bindings, nested variants, floating-point emission, and the main compiler's full ownership checks are outside its current subset. [Bootstrap sources](bootstrap/)

## Reproduce the self-build

Build the seed compiler from the same source revision, then run the self-build with a C23 compiler. The seed from alpha.2 predates this milestone.

```sh
go run ./cmd/gors build bootstrap/cgen/main.gors --backend c -o stage1
./stage1 emit bootstrap/cgen/main.gors -o stage2.c
gcc -std=c23 -O2 stage2.c -o stage2
./stage2 emit bootstrap/cgen/main.gors -o stage3.c
cmp stage2.c stage3.c
```

The generated C must match byte for byte. After building stage 1, the remaining commands run with Go absent from PATH.

Archives built from this revision include the verified `gors-bootstrap.c` and the `bootstrap/` sources. From an extracted archive, GCC is sufficient to build the compiler and regenerate its C:

```sh
gcc -std=c23 -O2 gors-bootstrap.c -o gors-bootstrap
./gors-bootstrap emit bootstrap/cgen/main.gors -o rebuilt.c
cmp gors-bootstrap.c rebuilt.c
```

## Build and run the bootstrap compiler

The release archives include standalone `gors-bootstrap.c`. In an extracted archive on Linux, the following commands build it and compile the file counter. On Windows, use `.exe` for the executable names.

```sh
gcc -std=c23 -O2 gors-bootstrap.c -o gors-bootstrap
./gors-bootstrap emit examples/wc.gors -o wc.c
gcc -std=c23 -O2 wc.c -o wc
./wc README.md
```

`wc` prints the number of newlines, the byte count, and the path. `emit -o` replaces the destination after successful compilation and writing. With no `-o`, it writes the generated C to stdout.

## Build the main compiler from source

```sh
go build -o gors ./cmd/gors
./gors --version
./gors run examples/modules/main.gors --backend c
./gors run examples/echo.gors --backend go -- "hello world" --verbose
./gors emit examples/fib.gors --backend c
```

Building from source and compiling with the Go backend require Go 1.22+. The C backend requires a C23 compiler. The CLI defaults to C if it finds a C compiler and falls back to Go otherwise. Use `gors build <file> -o <binary>` to keep an executable and `--` to pass arguments to a program run with `gors run`.

## Development and feedback

```sh
go test ./...
go vet ./...
go run ./tools/package -out dist
```

Tests build and run programs through both backends. They check output, exit codes, ownership diagnostics, and the bootstrap's generated C. The full test suite requires Go and GCC with C23 support.

Packaging also requires a C23 compiler. It compares C emitted by consecutive bootstrap stages before writing either archive. Use `-cc` to select the compiler executable.

If you try a release and find a bug or a confusing diagnostic, share a small `.gors` reproducer with your OS, backend, and `gors --version`. [Open an issue](https://github.com/rxzsu/Gors/issues/new)

## License

GoRs is available under the [MIT License](LICENSE).
