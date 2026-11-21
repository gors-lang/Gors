# Changelog

## 1.0.0-alpha.2

- Added `write_text(path, text)` to both main backends and the bootstrap compiler. `gors-bootstrap emit <file> -o <output.c>` writes generated C directly to a file.
- The bootstrap type checker and C emitter support `Result<T, E>`, the `Ok` and `Err` constructors, and exhaustive `match`.
- `read_bytes` reads files in binary mode. The bootstrap compiler builds `wc.gors`, with tests covering empty, binary, and missing files.

## 1.0.0-alpha.1

- The bootstrap compiler supports `[]int`, `[]bool`, and `[]string`, including literals, iteration, element assignment, and `push`. Slicing `[]int` and `[]bool` produces an independent copy.
- Linux and Windows archives include standalone `gors-bootstrap.c` and example programs. The release workflow publishes tags with version suffixes as prereleases.
- The GoRs C emitter supports strings with explicit lengths and `main(args: []string)`, including indexing and `for` iteration.
- Added the first C23 emitter written in GoRs. The `emit` command compiles a subset with `int`, `bool`, functions, and `while`. End-to-end tests compile the generated C and compare it with the main C backend.
- Added an experimental single-file semantic checker written in GoRs to check local names, calls, primitive types, slices, and function returns.
- The GoRs parser builds syntax trees for function and method bodies. Tests cover tree shape, syntax errors, and the source corpus on both backends.
- Fixed name resolution in `for` expressions and slice bounds in file modules.
- The Go backend stores a temporary slice in a local variable before iterating over it with `for`.
- The GoRs parser gained support for struct declarations, function signatures, and method signatures, with byte ranges for function bodies.
- The Go backend uses checked types for struct fields, including `Option` and `Result`.
- The GoRs lexer exposes source bytes and structured tokens through `scan`. The import parser uses it to parse leading `import` declarations.
- Added a GoRs lexer in `bootstrap/lexer` that prints token kinds, positions, and byte ranges. Tests compare it with the main lexer on both backends.
- Fixed the C backend's handling of borrowed values read through dereferencing. Moving an owning value through `*` is rejected.
- Added binary file input through `read_bytes(path)`, stderr output through `eprintln`, and exit codes through `main -> int`. The `examples/wc.gors` example uses all three.
- Both backends accept command-line arguments through `fn main(args: []string)`. `gors run` passes everything after `--` to the program.
- Slicing copyable elements creates independent storage in both backends.
- The C and Go backends report the same bounds diagnostics for string and slice ranges.
- Rejects slicing non-copyable elements with `E183` to prevent double frees from shallow copies in C.
- The source value is protected from modification and moves while evaluating slice bounds. Empty slices avoid null-pointer arithmetic in C.
- Clarified lifetime elision for returned references.

## 0.1.0

- Added local file modules with `import alias "file.gors"`, `alias::name`, and `pub`.
- Added exhaustive `match` for `Option` and `Result`, with ownership transferred into the selected arm.
- Added cleanup for temporaries, remaining fields of partially moved structs, replaced values, and arguments evaluated before early return through `?`.
- Checked references and aliases, call arguments, methods, loops, and branches. Documented the v0.1 lifetime restrictions.
- Added diagnostics with source text, highlights, related locations, and hints.
- Fixed short-circuit evaluation, loop conditions, and `?` with differing success types in both backends. Unified bounds errors.
- Added `gors --version`, the language reference, Linux and Windows CI, and release archives with SHA-256 checksums.
