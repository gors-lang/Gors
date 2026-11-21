# GoRs v0.1 language reference

This reference describes the experimental v0.1 language used by the **1.0.0-alpha.2** CLI. The main compiler is written in Go. Its C23 and Go backends share the same type and ownership checks.

[Русская версия](language-v0.1.ru.md)

## Running the compiler

Building the compiler requires Go 1.22+. The C backend also needs GCC 14+ or a compatible Clang with C23 support.

```sh
go build -o gors ./cmd/gors
./gors --version
./gors run examples/modules/main.gors --backend c
./gors run examples/modules/main.gors --backend go
./gors build examples/modules/main.gors -o app --backend c
./gors emit examples/modules/main.gors --backend go
```

The `run`, `build`, and `emit` commands take one entry file and load its imports. `build` writes a binary to the path given by `-o`. `run` uses a temporary directory. The CLI defaults to C if it finds a C compiler and uses Go otherwise. An unknown backend is an error. `--opt=-O3` sets the C compiler's optimization flag.

## Files and visibility

```rust
// math.gors
pub struct Point { x: int, y: int }
fn square(x: int) -> int { return x * x }
pub fn (p: &Point) norm2() -> int { return square(p.x) + square(p.y) }

// main.gors
import math "math.gors"
fn main() {
    let p = math::Point { x: 3, y: 4 }
    println(p.norm2())
}
```

Each file is a module. `import alias "relative/path.gors"` resolves relative to the importing file. Absolute paths and import cycles are errors. The compiler loads each physical file once and gives each module its own global namespace. Every module declares its dependencies explicitly.

Functions, structs, and methods are private by default. `pub` makes a declaration available to other modules. Use `alias::name` for an imported function or type and `value.method()` for a method call. All fields of an exported struct are accessible.

Modules contain declarations and imports. Loading one does not execute code. Diagnostics for an imported file include its source text.

The entry file's `main` takes either zero parameters or one `[]string` parameter. This parameter owns the command-line arguments. It excludes the executable name and is empty when no arguments are supplied. With `gors run`, pass program arguments after `--`. On Windows, the C backend receives arguments in the C runtime's encoding.

`main` may return an `int`. The low eight bits become the process exit status. A `main` with no return value exits with status 0.

### File I/O and diagnostics

- `read_bytes(path)` reads the whole file and returns `Result<[]int, string>`. Each element in `Ok` is a byte from 0 to 255. If opening or reading the file fails, the function returns `Err("could not read file")`. It borrows the path for the duration of the call.
- `write_text(path, text)` writes a string without appending a newline and returns `Result<bool, string>`. It borrows both arguments. It writes a temporary file before replacing the destination. Success returns `Ok(true)`. Failure returns `Err("could not write file")`. The main C backend uses null-terminated strings. The Go backend and bootstrap compiler preserve embedded null bytes.
- `eprintln(...)` writes its arguments and a newline to stderr.
- `bytes_to_string(values)` borrows a `[]int` and returns its bytes as a string. Each element must be in the range 1 through 255. An invalid byte exits with status 1.

## Types and expressions

GoRs supports 64-bit `int` and `float`, `bool`, `string`, structs, `[]T`, `Option<T>`, `Result<T, E>`, and local references `&T` and `&mut T`.

Expressions include arithmetic, comparisons, `&&`, `||`, `!`, field access, calls, indexing, and string concatenation with `+`. `&&` and `||` evaluate the right operand only when needed. Call arguments evaluate left to right, including when an argument uses `?`.

Recursive types, user-defined generics, and implicit conversions are unavailable in v0.1. Integer overflow behavior and exact floating-point output formatting can vary by backend.

```rust
fn sum(xs: []int) -> int {
    let mut total = 0
    let mut i = 0
    while i < len(xs) {
        total = total + xs[i]
        i = i + 1
    }
    return total
}
```

Built-in operations include `println(...)`, `print(...)`, `len(value)`, and `push(&mut slice, value)`. Indexing a slice checks its bounds. An invalid index exits with status 1 and writes `gors: index N out of bounds (len M)` to stderr. A fatal runtime error exits immediately. Use successful executions for leak checks.

Slicing with `s[low:high]` produces an independent copy and leaves the original available. Omitted bounds default to 0 and `len(s)`. Slice elements must be copyable (`int`, `float`, or `bool`). Slicing owning elements such as strings, structs, nested slices, and variants produces `E183`. Strings use byte indices for slicing.

Both backends enforce `0 <= low <= high <= len(s)`. A violation exits with status 1 and writes `gors: slice bounds out of range [L:H] (len N)`.

## Control flow and variants

Statements include `let`, `let mut`, assignment, `if`/`else`, `while`, `for`, `break`, `continue`, `return`, `if let`, `match`, and nested blocks. A function with a return type must return a value on every path. Statements after an unconditional `return`, `break`, or `continue` are errors. `let _ = expression` evaluates and discards a value. The discard name is write-only.

```rust
fn greeting(ok: bool) -> Option<string> {
    if ok { return Some("hello") }
    return None
}
fn main() {
    match greeting(true) {
        Some(message) => { println(message) },
        None => { println("empty") },
    }
}
```

`match` works as a statement or an expression. It evaluates the subject once and consumes it. An `Option` match must cover `Some(binding)` and `None`. A `Result` match must cover `Ok(binding)` and `Err(binding)`. Arms can appear in any order. Duplicate, incompatible, or missing variants are errors.

An arm can be an expression or a block. Each arm of a match expression produces the same result type. `Some(_)`, `Ok(_)`, and `Err(_)` discard the payload. Nested patterns, guards, and a catch-all `_ =>` arm are unavailable.

`None` needs a type context. `Ok` and `Err` need a `Result<T, E>` context such as a local annotation or the function's return type. `value?` unwraps `Ok` and immediately returns the error on `Err`. The error types must match. The success types may differ.

## Ownership and references

`int`, `float`, and `bool` are copied. Strings, slices, structs, and variants move when assigned to a binding, passed by value, or returned. A moved place cannot be read. When a field or slice element moves out, the checker conservatively treats its containing value as moved.

Bindings are immutable unless declared with `mut`. A value can have multiple shared borrows or one exclusive borrow. Copying a shared reference creates an alias and extends the borrow until the last use of any alias. Passing `&mut` to a reference parameter temporarily reborrows it. `&mut` references cannot be copied. Conflicting references in one call's arguments are errors.

References may be locals or parameters, and functions and methods may return them under lifetime elision:

- A function that returns `&T` or `&mut T` must have exactly one reference parameter. A method with a reference receiver ties the returned reference to the receiver. A method with a value receiver needs exactly one reference parameter among its other arguments.
- The returned reference must come from that parameter or receiver. A local variable cannot supply a returned reference.
- At the call site, the borrow of the corresponding argument extends for the lifetime of the returned reference.
- Use unary `*` to dereference a reference, including as an assignment target when mutable.

References cannot be stored in struct fields, slices, `Option`, or `Result`. Moving an owning field through a reference is an error.

Local shadowing is prohibited. A repeating loop cannot move an owning value declared outside that loop. Create the value inside each iteration or pass it by reference. Borrows used again in the loop remain active across its back edge. The checker merges ownership states across `if`, `if let`, and `match` branches and can reject some safe programs.

The C backend drops owning values on normal scope exit, return, and error propagation through `?`. It drops temporaries after an expression and drops the old value of a binding after evaluating its replacement. After a partial move, it drops the remaining fields.

```sh
GORS_LEAKCHECK=1 ./app
```

## Testing and packaging

```sh
go test -count=1 ./...
go vet ./...
go run ./tools/package -out dist
```

Tests compile and run programs through both backends. They cover output, exit codes, diagnostics, ownership failures, and C leak accounting. The full test matrix requires Go and GCC with C23 support.

The packager requires Go and a C23 compiler. It builds two bootstrap stages and checks that both emit identical C. Both the Linux amd64 `.tar.gz` and Windows amd64 `.zip` include that verified `gors-bootstrap.c`, the bootstrap GoRs sources, the main CLI, examples, and documentation. `SHA256SUMS` records both archive checksums. The GitHub release workflow publishes version tags as releases and tags with prerelease suffixes as prereleases.
