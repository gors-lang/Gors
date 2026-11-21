package compiler_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"gors/internal/compiler"
)

func TestBootstrapCEmitter(t *testing.T) {
	t.Parallel()

	entry := filepath.Join("..", "..", "bootstrap", "cgen", "main.gors")
	dir := t.TempDir()
	write := func(name, source string) string {
		path := filepath.Join(dir, name+".gors")
		if err := os.WriteFile(path, []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	cases := []struct {
		name, path, stdout, stderr string
		status                     int
		args                       []string
	}{
		{"fib", filepath.Join("..", "..", "examples", "fib.gors"), "0\n1\n1\n2\n3\n5\n8\n13\n21\n34\n", "", 0, nil},
		{"hello", filepath.Join("..", "..", "examples", "hello.gors"), "hello from GoRs\n", "", 0, nil},
		{"Point by value", filepath.Join("..", "..", "examples", "point.gors"), "25\n", "", 0, nil},
		{"struct slices and variants", write("struct slices and variants", `
			struct Item { value: int }
			struct Failure { message: string }
			fn append(items: &mut []Item, value: int) { push(items, Item { value: value }) }
			fn list(ok: bool) -> Result<[]Item, Failure> {
				if !ok { return Err(Failure { message: "unavailable" }) }
				let mut items = [Item { value: 2 }]
				append(&mut items, 3)
				return Ok(items)
			}
			fn first(ok: bool) -> Result<Item, Failure> {
				let items = list(ok)?
				return Ok(items[0])
			}
			fn main() {
				match list(true) {
					Ok(items) => { for item in items { println(item.value) } },
					Err(issue) => { eprintln(issue.message) },
				}
				match first(false) {
					Ok(_) => { println("unexpected") },
					Err(issue) => { eprintln(issue.message) },
				}
				let optional = Some(Item { value: -4 })
				match optional {
					Some(item) => { println(item.value) },
					None => { println("unexpected") },
				}
			}`), "2\n3\n-4\n", "unavailable\n", 0, nil},
		{"explicit reference parameters", filepath.Join("..", "..", "examples", "structs.gors"), "25\n4\n5\n", "", 0, nil},
		{"method receivers", filepath.Join("..", "..", "examples", "methods.gors"), "25\n4\n5\n", "", 0, nil},
		{"file modules", filepath.Join("..", "..", "examples", "modules", "main.gors"), "25\nhello\n", "", 0, nil},
		{"Option expression and bytes", write("Option expression and bytes", `
			fn find(ok: bool) -> Option<string> {
				if ok { return Some(bytes_to_string([71, 111])) }
				return None
			}
			fn render(value: Option<string>) -> string {
				return match value { None => "empty", Some(text) => text }
			}
			fn main() { println(render(find(true)), render(find(false))) }
		`), "Go empty\n", "", 0, nil},
		{"method namespaces", write("method namespaces", `
			struct A { x: int }
			struct B { x: int }
			struct Wrap { inner: A }
			fn (a: &mut A) bump() { a.x = a.x + 1 }
			fn (a: A) value() -> int { return a.x }
			fn (b: &B) value() -> int { return b.x * 2 }
			fn main() {
				let mut w = Wrap { inner: A { x: 3 } }
				w.inner.bump()
				let b = B { x: 5 }
				println(w.inner.value(), b.value())
			}`), "4 10\n", "", 0, nil},
		{"nested structs and mutable fields", write("nested structs", `
			struct Outer { inner: Inner, values: []int }
			struct Inner { number: int, text: string }
			fn make() -> Outer {
				return Outer { values: [1, 2], inner: Inner { text: "old", number: 3 } }
			}
			fn main() {
				let mut x = make()
				x.inner.number = 9
				x.inner.text = "new"
				x.values[0] = 7
				println(x.inner.number, x.inner.text, x.values[0])
			}`), "9 new 7\n", "", 0, nil},
		{"echo", filepath.Join("..", "..", "examples", "echo.gors"), "hello world --flag\n", "", 0, []string{"hello world", "--flag"}},
		{"empty args", filepath.Join("..", "..", "examples", "echo.gors"), "\n", "", 0, nil},
		{"slices example", filepath.Join("..", "..", "examples", "slices.gors"), "4\n10\n4\n40\n99\n", "", 0, nil},
		{"subslice example", filepath.Join("..", "..", "examples", "subslice.gors"), "Full slice len:\n5\nSubslice [1:4] len:\n3\n20\n30\n40\nHead [:2] len:\n2\n10\n20\nTail [3:] len:\n2\n40\n50\nHello\nGoRs!\n", "", 0, nil},
		{"slice mutation", write("slice mutation", `
			fn empty() -> []int { return [] }
			fn count(xs: []int) -> int { return len(xs) }
			fn main() {
				let mut xs = empty()
				let mut i = 0
				while i < 17 { push(&mut xs, i); i = i + 1 }
				let mut copy = xs[1:3]
				xs[1] = 99
				copy[0] = 4
				println(len(xs), xs[1], xs[16], copy[0], copy[1], count([]))
				xs = ([])
				println(len(xs))
			}`), "17 99 16 4 2 0\n0\n", "", 0, nil},
		{"bool and string slices", write("bool and string slices", `
			fn main() {
				let mut flags: []bool = []
				push(&mut flags, false); flags[0] = true
				let mut words = ["a", "b"]
				push(&mut words, "c"); words[1] = "B"
				println(flags[0], len(words))
				for word in words { println(word) }
			}`), "true 3\na\nB\nc\n", "", 0, nil},
		{"Result constructors and expression match", write("result", `
			fn value(ok: bool) -> Result<[]int, string> {
				if ok { let items: []int = []; return Ok(items) }
				return Err("bad")
			}
			fn count(result: Result<[]int, string>) -> int {
				return match result {
					Err(message) => len(message),
					Ok(items) => { let n = len(items); n + 1 },
				}
			}
			fn main() { println(count(value(true)), count(value(false))) }
		`), "1 3\n", "", 0, nil},
		{"call order", write("call order", `
			fn trace(n: int) -> int { println(n); return n }
			fn flag(n: int) -> bool { println(n); return true }
			fn join(a: int, b: int) -> int { return a + b }
			fn main() {
				println(join(trace(1), trace(2)), trace(3))
				println(false && flag(9), true || flag(8), true && flag(4))
			}`), "1\n2\n3\n3 3\n4\nfalse true true\n", "", 0, nil},
		{"loops and status", write("loops and status", `
			fn main() -> int {
				let mut i = 00
				while i < 10 {
					i = i + 1
					if i == 2 { continue }
					if i == 4 { break }
					println(i, i < 2)
				}
				{ let игрок = 0_8; println(игрок) }
				eprintln(!false)
				return 261
			}`), "1 true\n3 false\n8\n", "true\n", 5, nil},
		{"forward functions and discard", write("forward functions and discard", `
			fn main() { let _ = test(1, 2); println(test(3, 4)) }
			fn test(_: int, _: int) -> bool { return true }`), "true\n", "", 0, nil},
		{"string operations", write("string operations", `
			fn title(s: string) -> string { return ">" + s }
			fn main(args: []string) -> int {
				let count = len(args)
				let joined = title(args[0]) + "!"
				println(joined[:2], len(joined), count)
				println(joined == ">Go!", joined != ">Go")
				println("a\tb", "quote:\"")
				return count
			}`), ">G 4 1\ntrue true\na\tb quote:\"\n", "", 1, []string{"Go"}},
		{"argument bounds", write("argument bounds", `fn main(args: []string) { println(args[1]) }`),
			"", "gors: index 1 out of bounds (len 1)\n", 1, []string{"one"}},
		{"string slice bounds", write("string slice bounds", `fn main() { println("ab"[:3]) }`),
			"", "gors: slice bounds out of range [0:3] (len 2)\n", 1, nil},
	}
	bad := []struct{ name, path, message string }{
		{"type error", write("type error", "fn main() { let x: bool = 1 }"), "initializer type mismatch"},
		{"unsupported expression", write("unsupported expression", "fn main() { println(1.5) }"), "outside the C emitter subset"},
		{"overflowing literal", write("overflowing literal", "fn main() { println(9223372036854775808) }"), "integer literal exceeds int"},
		{"nested slices", write("nested slices", "fn main() { let xs = [[1]] }"), "nested slices are outside"},
		{"immutable push", write("immutable push", "fn main() { let xs = [1]; push(&mut xs, 2) }"), "assignment to immutable name"},
		{"push type", write("push type", "fn main() { let mut xs = [1]; push(&mut xs, true) }"), "push element type mismatch"},
		{"index assignment type", write("index assignment type", "fn main() { let mut xs = [1]; xs[0] = true }"), "assignment type mismatch"},
		{"constructor without context", write("constructor without context", "fn main() { let x = Ok(1) }"), "Result constructor needs a Result type context"},
		{"wrong Result payload", write("wrong Result payload", `fn main() { let x: Result<int, string> = Err(false) }`), "Result payload type mismatch"},
		{"nonexhaustive match", write("nonexhaustive match", `fn main() { let x: Result<int, string> = Ok(1); match x { Ok(i) => println(i) } }`), "match must cover both Ok and Err"},
		{"duplicate match arm", write("duplicate match arm", `fn main() { let x: Result<int, string> = Ok(1); match x { Ok(i) => println(i), Ok(j) => println(j) } }`), "duplicate Result match arm"},
		{"mismatched arms", write("mismatched arms", `fn main() { let x: Result<int, string> = Ok(1); let y = match x { Ok(i) => i, Err(e) => e } }`), "match arms have different types"},
		{"invalid read path type", write("invalid read path type", `fn main() { let x = read_bytes(1) }`), "read_bytes expects a string path"},
		{"duplicate struct names", write("duplicate struct names", `struct A {} struct A {} fn main() {}`), "duplicate declaration name"},
		{"duplicate field declaration", write("duplicate field declaration", `struct A { x: int, x: bool } fn main() {}`), "duplicate struct field declaration"},
		{"recursive struct layout", write("recursive struct layout", `struct A { b: B } struct B { a: A } fn main() {}`), "recursive struct layout"},
		{"missing field", write("missing field", `struct A { x: int } fn main() { let a = A {} }`), "missing struct field initializer"},
		{"unknown field", write("unknown field", `struct A { x: int } fn main() { let a = A { y: 1 } }`), "unknown struct field"},
		{"duplicate initializer", write("duplicate initializer", `struct A { x: int } fn main() { let a = A { x: 1, x: 2 } }`), "duplicate struct field initializer"},
		{"wrong field type", write("wrong field type", `struct A { x: int } fn main() { let a = A { x: false } }`), "struct field type mismatch"},
		{"immutable field assignment", write("immutable field assignment", `struct A { x: int } fn main() { let a = A { x: 1 }; a.x = 2 }`), "assignment to immutable name"},
		{"immutable receiver", write("immutable receiver", `struct A { x: int } fn (p: &mut A) set() { p.x = 2 } fn main() { let a = A { x: 1 }; a.set() }`), "assignment to immutable name"},
		{"shared receiver write", write("shared receiver write", `struct A { x: int } fn (p: &A) set() { p.x = 2 } fn main() { }`), "assignment to immutable name"},
		{"conflicting call borrows", write("conflicting call borrows", `struct A { x: int } fn f(p: &mut A, q: &A) {} fn main() { let mut a = A { x: 1 }; f(&mut a, &a) }`), "conflicting borrows in call"},
		{"conflicting receiver borrow", write("conflicting receiver borrow", `struct A { x: int } fn (p: &mut A) f(q: &A) {} fn main() { let mut a = A { x: 1 }; a.f(&a) }`), "conflicting borrows in call"},
		{"escaping reference", write("escaping reference", `struct A { x: int } fn f(p: &A) -> &A { return p } fn main() {}`), "returning references is outside"},
		{"nonexhaustive Option", write("nonexhaustive Option", `fn main() { match Some(1) { Some(x) => println(x) } }`), "match must cover both Some and None"},
		{"wrong try error type", write("wrong try error type", `
			fn source() -> Result<int, string> { return Err("bad") }
			fn wrong() -> Result<int, bool> { let x = source()?; return Ok(x) }
			fn main() {}
		`), "? error type differs"},
		{"recursive struct slice", write("recursive struct slice", `struct Tree { children: []Tree } fn main() {}`), "recursive struct layout"},
	}
	wc := filepath.Join("..", "..", "examples", "wc.gors")
	empty, binary, missing := filepath.Join(dir, "empty.txt"), filepath.Join(dir, "binary input.bin"), filepath.Join(dir, "missing.txt")
	if err := os.WriteFile(empty, nil, 0600); err != nil {
		t.Fatal(err)
	}
	// Cross more than one fread buffer and cover null, CRLF and non-UTF-8.
	contents := bytes.Repeat([]byte{0, 13, 10, 255, 10}, 1000)
	if err := os.WriteFile(binary, contents, 0600); err != nil {
		t.Fatal(err)
	}
	for _, backend := range []compiler.Backend{compiler.BackendC, compiler.BackendGo} {
		t.Run(string(backend), func(t *testing.T) {
			emitter := buildBootstrapTool(t, entry, backend)
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					out, diagnostics := runBootstrapCommand(t, emitter, 0, "emit", tc.path)
					if diagnostics != "" || !strings.Contains(out, "int main(") {
						t.Fatalf("invalid emitter result: %q\n%s", diagnostics, out)
					}
					binary := buildEmittedC(t, out)
					stdout, stderr := runBootstrapCommand(t, binary, tc.status, tc.args...)
					if stdout != tc.stdout || stderr != tc.stderr {
						t.Fatalf("got stdout %q stderr %q, want stdout %q stderr %q", stdout, stderr, tc.stdout, tc.stderr)
					}
					reference := buildBootstrapTool(t, tc.path, compiler.BackendC)
					// Fatal runtime diagnostics abort without unwinding on the existing C backend.
					leakcheck := !strings.HasPrefix(tc.stderr, "gors:")
					wantOut, wantErr := runBootstrapCommandWithLeakCheck(t, reference, tc.status, leakcheck, tc.args...)
					if stdout != wantOut || stderr != wantErr {
						t.Fatalf("existing C backend gave stdout %q stderr %q", wantOut, wantErr)
					}
				})
			}
			t.Run("embedded null", func(t *testing.T) {
				path := write("embedded null "+string(backend), `fn main() { let s = "a\0b"; println(len(s), s[1:]) }`)
				source, diagnostics := runBootstrapCommand(t, emitter, 0, "emit", path)
				if diagnostics != "" {
					t.Fatalf("emit diagnostics: %s", diagnostics)
				}
				out, stderr := runBootstrapCommand(t, buildEmittedC(t, source), 0)
				if out != "3 \x00b\n" || stderr != "" {
					t.Fatalf("got stdout %q stderr %q", out, stderr)
				}
			})
			t.Run("wc file scenarios", func(t *testing.T) {
				source, diagnostics := runBootstrapCommand(t, emitter, 0, "emit", wc)
				if diagnostics != "" {
					t.Fatalf("emit diagnostics: %s", diagnostics)
				}
				exe := buildEmittedC(t, source)
				reference := buildBootstrapTool(t, wc, compiler.BackendC)
				for _, tc := range []struct {
					name, path, stdout, stderr string
					status                     int
				}{
					{"empty", empty, "0 0 " + empty + "\n", "", 0},
					{"binary", binary, "2000 5000 " + binary + "\n", "", 0},
					{"missing", missing, "", "wc: " + missing + " could not read file\n", 1},
					{"usage", "", "", "usage: wc <file>\n", 2},
				} {
					t.Run(tc.name, func(t *testing.T) {
						var args []string
						if tc.path != "" {
							args = []string{tc.path}
						}
						stdout, stderr := runBootstrapCommand(t, exe, tc.status, args...)
						if stdout != tc.stdout || stderr != tc.stderr {
							t.Fatalf("got stdout %q stderr %q, want stdout %q stderr %q", stdout, stderr, tc.stdout, tc.stderr)
						}
						wantOut, wantErr := runBootstrapCommand(t, reference, tc.status, args...)
						if stdout != wantOut || stderr != wantErr {
							t.Fatalf("existing C backend gave stdout %q stderr %q", wantOut, wantErr)
						}
					})
				}
			})
			for _, tc := range bad {
				t.Run(tc.name, func(t *testing.T) {
					out, diagnostics := runBootstrapCommand(t, emitter, 1, "emit", tc.path)
					if out != "" || !strings.Contains(diagnostics, tc.message) || strings.Contains(diagnostics, "leaked allocation") {
						t.Fatalf("got stdout %q stderr %q, expected only diagnostic %q", out, diagnostics, tc.message)
					}
				})
			}
		})
	}
}

func TestBootstrapFileModules(t *testing.T) {
	entry := filepath.Join("..", "..", "bootstrap", "cgen", "main.gors")
	directory := t.TempDir()
	write := func(name, input string) string {
		t.Helper()
		path := filepath.Join(directory, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(input), 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	write("common.gors", `pub fn number() -> int { return 7 }`)
	write("left/l.gors", `
		import common "../common.gors"
		pub struct Point { x: int }
		struct Hidden { x: int }
		fn helper() -> int { return 2 }
		pub fn number() -> int { return helper() + common::number() }
		fn (p: &Point) hidden() -> int { return p.x }
		pub fn (p: &Point) value() -> int { return p.hidden() }
	`)
	write("right/r.gors", `import common ".././common.gors" fn helper() -> int { return 3 }
		pub fn number() -> int { return helper() + common::number() }`)
	success := write("main.gors", `
		import left "left/l.gors"
		import right "right/r.gors"
		import other "left/../left/l.gors"
		struct Wrap { p: left::Point }
		fn main() {
			let w = Wrap { p: other::Point { x: 4 } }
			println(w.p.value(), left::number(), right::number())
		}
	`)
	write("cycle/other.gors", `import root "../cycle.gors"`)
	bad := []struct{ name, path, diagnostic string }{
		{"private function", write("private-fn.gors", `import left "left/l.gors" fn main() { println(left::helper()) }`), "imported declaration is private"},
		{"private type", write("private-type.gors", `import left "left/l.gors" fn main() { let x = left::Hidden { x: 1 } }`), "imported declaration is private"},
		{"private method", write("private-method.gors", `import left "left/l.gors" fn main() { let p = left::Point { x: 1 }; println(p.hidden()) }`), "imported method is private"},
		{"unknown alias", write("unknown.gors", `fn main() { println(foo::bar()) }`), "unknown import alias"},
		{"cyclic import", write("cycle.gors", `import dep "cycle/other.gors" fn main() {}`), "cyclic import"},
		{"duplicate alias", write("duplicate.gors", `import a "common.gors" import a "left/l.gors" fn main() {}`), "duplicate import alias"},
		{"absolute import", write("absolute.gors", `import a "/common.gors" fn main() {}`), "import requires a relative file path"},
		{"missing file", write("missing.gors", `import a "missing-module.gors" fn main() {}`), "could not read module"},
	}
	for _, backend := range []compiler.Backend{compiler.BackendC, compiler.BackendGo} {
		t.Run(string(backend), func(t *testing.T) {
			generator := buildBootstrapTool(t, entry, backend)
			code, diagnostics := runBootstrapCommand(t, generator, 0, "emit", success)
			if diagnostics != "" {
				t.Fatalf("emit: %s", diagnostics)
			}
			out, stderr := runBootstrapCommand(t, buildEmittedC(t, code), 0)
			want, refErr := runBootstrapCommand(t, buildBootstrapTool(t, success, compiler.BackendC), 0)
			if out != "4 9 10\n" || out != want || stderr != "" || refErr != "" {
				t.Fatalf("bootstrap stdout %q stderr %q, reference stdout %q stderr %q", out, stderr, want, refErr)
			}
			for _, tc := range bad {
				t.Run(tc.name, func(t *testing.T) {
					stdout, stderr := runBootstrapCommand(t, generator, 1, "emit", tc.path)
					if stdout != "" || !strings.Contains(stderr, tc.diagnostic) || strings.Contains(stderr, "leaked allocation") {
						t.Fatalf("got stdout %q stderr %q, expected diagnostic %q", stdout, stderr, tc.diagnostic)
					}
				})
			}
		})
	}
}

func TestBootstrapSelfHost(t *testing.T) {
	entry := filepath.Join("..", "..", "bootstrap", "cgen", "main.gors")
	stage1 := buildBootstrapTool(t, entry, compiler.BackendC)
	temp := t.TempDir()
	stage2Source := filepath.Join(temp, "stage2.c")
	stage3Source := filepath.Join(temp, "stage3.c")
	emit := func(binary, input, output string) []byte {
		t.Helper()
		stdout, stderr := runBootstrapCommand(t, binary, 0, "emit", input, "-o", output)
		if stdout != "" || stderr != "" {
			t.Fatalf("emit %s: stdout %q stderr %q", input, stdout, stderr)
		}
		data, err := os.ReadFile(output)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	first := emit(stage1, entry, stage2Source)
	stage2 := buildEmittedC(t, string(first))
	second := emit(stage2, entry, stage3Source)
	if !bytes.Equal(first, second) {
		t.Fatalf("self-compiled compiler emitted different C: %d vs %d bytes", len(first), len(second))
	}
	stage3 := buildEmittedC(t, string(second))
	example := filepath.Join("..", "..", "examples", "modules", "main.gors")
	code := emit(stage3, example, filepath.Join(temp, "example.c"))
	out, errOut := runBootstrapCommand(t, buildEmittedC(t, string(code)), 0)
	if out != "25\nhello\n" || errOut != "" {
		t.Fatalf("self-compiled compiler's program gave stdout %q stderr %q", out, errOut)
	}
	echo := filepath.Join("..", "..", "examples", "echo.gors")
	out, errOut = runBootstrapCommand(t, stage3, 0, "run", echo, "--", "quoted \"text\"", "", "trailing\\")
	if out != "quoted \"text\"  trailing\\\n" || errOut != "" {
		t.Fatalf("self-compiled compiler run gave stdout %q stderr %q", out, errOut)
	}
}

func TestBootstrapNativeCLI(t *testing.T) {
	entry := filepath.Join("..", "..", "bootstrap", "cgen", "main.gors")
	example := func(name string) string { return filepath.Join("..", "..", "examples", name+".gors") }
	for _, backend := range []compiler.Backend{compiler.BackendC, compiler.BackendGo} {
		t.Run(string(backend), func(t *testing.T) {
			cli := buildBootstrapTool(t, entry, backend)
			dir := t.TempDir()
			output := filepath.Join(dir, "fib output")
			if runtime.GOOS == "windows" {
				output += ".exe"
			}
			if err := os.WriteFile(output, []byte("existing output"), 0600); err != nil {
				t.Fatal(err)
			}
			out, errOut := runBootstrapCommand(t, cli, 0, "build", example("fib"), "-o", output)
			if out != "" || errOut != "" {
				t.Fatalf("build gave stdout %q stderr %q", out, errOut)
			}
			out, errOut = runBootstrapCommand(t, output, 0)
			if out != "0\n1\n1\n2\n3\n5\n8\n13\n21\n34\n" || errOut != "" {
				t.Fatalf("fib gave stdout %q stderr %q", out, errOut)
			}
			before, err := os.ReadFile(output)
			if err != nil {
				t.Fatal(err)
			}
			bad := filepath.Join(dir, "bad.gors")
			if err := os.WriteFile(bad, []byte("fn main() { let x: int = true }"), 0600); err != nil {
				t.Fatal(err)
			}
			out, errOut = runBootstrapCommand(t, cli, 1, "build", bad, "-o", output)
			if out != "" || !strings.Contains(errOut, "initializer type mismatch") {
				t.Fatalf("bad build gave stdout %q stderr %q", out, errOut)
			}
			if after, err := os.ReadFile(output); err != nil || !bytes.Equal(before, after) {
				t.Fatalf("failed build modified its output: %v", err)
			}
			out, errOut = runBootstrapCommand(t, cli, 0, "run", example("echo"), "--", "hello world", "--flag", "", `a\"b`, `tail\`)
			if out != "hello world --flag  a\\\"b tail\\\n" || errOut != "" {
				t.Fatalf("echo gave stdout %q stderr %q", out, errOut)
			}
			input := filepath.Join(dir, "binary input")
			if err := os.WriteFile(input, []byte{0, 10, 255}, 0600); err != nil {
				t.Fatal(err)
			}
			out, errOut = runBootstrapCommand(t, cli, 0, "run", example("wc"), "--", input)
			if out != "1 3 "+input+"\n" || errOut != "" {
				t.Fatalf("wc gave stdout %q stderr %q", out, errOut)
			}
			missing := input + ".missing"
			out, errOut = runBootstrapCommand(t, cli, 1, "run", example("wc"), "--", missing)
			if out != "" || errOut != "wc: "+missing+" could not read file\n" {
				t.Fatalf("wc error gave stdout %q stderr %q", out, errOut)
			}
			files, err := os.ReadDir(dir)
			if err != nil || len(files) != 3 {
				t.Fatalf("unexpected native build workspace: %v, %v", files, err)
			}
		})
	}
}

func buildEmittedC(t *testing.T, source string) string {
	t.Helper()
	dir := t.TempDir()
	file, bin := filepath.Join(dir, "main.c"), filepath.Join(dir, "program")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	if err := os.WriteFile(file, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if out, err := exec.CommandContext(ctx, "gcc", "-std=c23", "-pedantic-errors", "-O2", file, "-o", bin).CombinedOutput(); err != nil {
		t.Fatalf("gcc: %v\n%s\n%s", err, out, source)
	}
	return bin
}

func TestBootstrapFileOutput(t *testing.T) {
	entry := filepath.Join("..", "..", "bootstrap", "cgen", "main.gors")
	for _, backend := range []compiler.Backend{compiler.BackendC, compiler.BackendGo} {
		t.Run(string(backend), func(t *testing.T) {
			dir := t.TempDir()
			generator := buildBootstrapTool(t, entry, backend)
			input := filepath.Join("..", "..", "examples", "fib.gors")
			output := filepath.Join(dir, "fib output.c")
			if err := os.WriteFile(output, []byte("old contents"), 0600); err != nil {
				t.Fatal(err)
			}
			want, diagnostics := runBootstrapCommand(t, generator, 0, "emit", input)
			if diagnostics != "" {
				t.Fatalf("emit: %s", diagnostics)
			}
			stdout, stderr := runBootstrapCommand(t, generator, 0, "emit", input, "-o", output)
			if stdout != "" || stderr != "" {
				t.Fatalf("got stdout %q stderr %q", stdout, stderr)
			}
			source, err := os.ReadFile(output)
			if err != nil || string(source) != want {
				t.Fatalf("C file differs from stdout: %v", err)
			}
			got, diagnostics := runBootstrapCommand(t, buildEmittedC(t, string(source)), 0)
			if got != "0\n1\n1\n2\n3\n5\n8\n13\n21\n34\n" || diagnostics != "" {
				t.Fatalf("generated fib gave stdout %q stderr %q", got, diagnostics)
			}

			bad := filepath.Join(dir, "bad.gors")
			if err := os.WriteFile(bad, []byte("fn main() { let x: int = false }"), 0600); err != nil {
				t.Fatal(err)
			}
			stdout, stderr = runBootstrapCommand(t, generator, 1, "emit", bad, "-o", output)
			if stdout != "" || !strings.Contains(stderr, "initializer type mismatch") {
				t.Fatalf("got stdout %q stderr %q", stdout, stderr)
			}
			if data, err := os.ReadFile(output); err != nil || !bytes.Equal(data, source) {
				t.Fatalf("compilation failure changed the output file: %v", err)
			}

			destination := filepath.Join(dir, "destination")
			if err := os.Mkdir(destination, 0700); err != nil {
				t.Fatal(err)
			}
			stdout, stderr = runBootstrapCommand(t, generator, 2, "emit", input, "-o", destination)
			if stdout != "" || !strings.Contains(stderr, "could not write file") {
				t.Fatalf("got stdout %q stderr %q", stdout, stderr)
			}
			if info, err := os.Stat(destination); err != nil || !info.IsDir() {
				t.Fatalf("could not preserve destination directory: %v", err)
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 3 {
				t.Fatalf("unexpected files after write: %v", entries)
			}
		})
	}
}

func TestWriteText(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "writer.gors")
	if err := os.WriteFile(source, []byte(`
		fn main(args: []string) -> int {
			let path = args[0]
			let text = "line\nsecond"
			match write_text(path, text) {
				Ok(ok) => { println(ok, len(text)); return 0 }
				Err(issue) => { eprintln(issue); return 1 }
			}
		}`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, backend := range []compiler.Backend{compiler.BackendC, compiler.BackendGo} {
		t.Run(string(backend), func(t *testing.T) {
			primary := buildBootstrapTool(t, source, backend)
			entry := filepath.Join("..", "..", "bootstrap", "cgen", "main.gors")
			generator := buildBootstrapTool(t, entry, backend)
			code, diagnostics := runBootstrapCommand(t, generator, 0, "emit", source)
			if diagnostics != "" {
				t.Fatalf("emit: %s", diagnostics)
			}
			bootstrap := buildEmittedC(t, code)
			for _, program := range []string{primary, bootstrap} {
				target := filepath.Join(t.TempDir(), "written text.txt")
				got, stderr := runBootstrapCommand(t, program, 0, target)
				if got != "true 11\n" || stderr != "" {
					t.Fatalf("got stdout %q stderr %q", got, stderr)
				}
				if data, err := os.ReadFile(target); err != nil || string(data) != "line\nsecond" {
					t.Fatalf("write: %q, %v", data, err)
				}
				got, stderr = runBootstrapCommand(t, program, 1, filepath.Join(target, "bad"))
				if got != "" || stderr != "could not write file\n" {
					t.Fatalf("got stdout %q stderr %q", got, stderr)
				}
			}
		})
	}
}

func runBootstrapCommand(t *testing.T, bin string, wantExit int, args ...string) (string, string) {
	t.Helper()
	return runBootstrapCommandWithLeakCheck(t, bin, wantExit, true, args...)
}

func runBootstrapCommandWithLeakCheck(t *testing.T, bin string, wantExit int, leakcheck bool, args ...string) (string, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	flag := "0"
	if leakcheck {
		flag = "1"
	}
	cmd.Env = append(os.Environ(), "GORS_LEAKCHECK="+flag)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if err != nil {
		var exit *exec.ExitError
		if wantExit == 0 || !errors.As(err, &exit) || exit.ExitCode() != wantExit {
			t.Fatalf("run: %v\n%s\n%s", err, &stdout, &stderr)
		}
	} else if wantExit != 0 {
		t.Fatalf("expected exit status %d", wantExit)
	}
	return strings.ReplaceAll(stdout.String(), "\r\n", "\n"), strings.ReplaceAll(stderr.String(), "\r\n", "\n")
}
