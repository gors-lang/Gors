# Write your first GoRs command-line program

Follow the [quick start](../README.md#quick-start) to get `gors` and a C23 compiler.
Use the commands below from the extracted archive. On Windows, replace
`./gors` with `./gors.exe` and `./file-size` with `./file-size.exe`.

## Pass arguments to a program

```sh
./gors run examples/echo.gors --backend c -- "hello GoRs" --verbose
```

This prints `hello GoRs --verbose`. The first `--` separates compiler
options from program arguments. Quoting `"hello GoRs"` passes it as a single
element of the `[]string` that `main` receives.

## Read a file and handle errors

Save this as `file-size.gors`.

```rust
fn size(path: string) -> Result<int, string> {
    let bytes = read_bytes(path)?
    return Ok(len(bytes))
}

fn main(args: []string) -> int {
    if len(args) != 1 {
        eprintln("usage: file-size <path>")
        return 2
    }

    match size(args[0]) {
        Ok(count) => {
            println(count)
            return 0
        }
        Err(message) => {
            eprintln(message)
            return 1
        }
    }
}
```

`read_bytes` returns either file contents or an error string. The `?` operator
returns the error early from `size`. On success, `len(bytes)` gives the number of
bytes. In `main`, `match` handles both outcomes and selects an exit status.

```sh
./gors run file-size.gors --backend c -- README.md
./gors build file-size.gors --backend c -o file-size
./file-size README.md
```

Both runs print the byte count. Try a missing file to see the error on stderr and
exit status 1, or omit the filename to see the usage message and exit status 2.
On Windows, specify `-o file-size.exe` in the build command.

To inspect what the compiler sends to the C toolchain, run
`./gors emit file-size.gors --backend c`. This prints the generated C to stdout.
For a complete example that also counts newlines, see [`wc.gors`](../examples/wc.gors).
