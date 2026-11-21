#!/usr/bin/env bash
set -euo pipefail

archive=$(realpath "$1")
work=$(mktemp -d)
trap '/bin/rm -rf -- "$work"' EXIT
mkdir "$work/archive" "$work/toolchain"
for tool in gcc as ld; do
    ln -s "$(command -v "$tool")" "$work/toolchain/$tool"
done
tar -xf "$archive" -C "$work/archive"

cd "$work/archive"
export PATH="$work/toolchain"
if command -v go >/dev/null 2>&1; then
    echo "Go must be absent during the archive smoke test" >&2
    exit 1
fi
gcc -std=c23 -pedantic-errors -O2 gors-bootstrap.c -o gors-bootstrap
./gors-bootstrap emit bootstrap/cgen/main.gors -o reproduced.c
/usr/bin/cmp gors-bootstrap.c reproduced.c

./gors-bootstrap build examples/fib.gors -o 'fib output'
fib=$("./fib output")
test "$fib" = $'0\n1\n1\n2\n3\n5\n8\n13\n21\n34'
echoed=$(./gors-bootstrap run examples/echo.gors -- 'hello world' --flag '' 'quote:"tail\')
test "$echoed" = 'hello world --flag  quote:"tail\'
printf '\000\r\n\377\n' > 'binary input'
count=$(./gors-bootstrap run examples/wc.gors -- 'binary input')
test "$count" = '2 5 binary input'
status=0
message=$(./gors-bootstrap run examples/wc.gors -- 'missing file' 2>&1) || status=$?
test "$status" -eq 1
test "$message" = 'wc: missing file could not read file'
echo "Self-build, fib, echo, and wc passed with a GCC-only PATH"
