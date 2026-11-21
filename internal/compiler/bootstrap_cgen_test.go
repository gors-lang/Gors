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
