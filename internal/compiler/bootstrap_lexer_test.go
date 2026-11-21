package compiler_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"gors/internal/compiler"
	"gors/internal/lexer"
	"gors/internal/parser"
	"gors/internal/token"
)

func TestBootstrapLexer(t *testing.T) {
	entry := filepath.Join("..", "..", "bootstrap", "lexer", "main.gors")
	var files []string
	addSource := func(path string, item fs.DirEntry, err error) error {
		if err == nil && !item.IsDir() && filepath.Ext(path) == ".gors" {
			files = append(files, path)
		}
		return err
	}
	for _, dir := range []string{filepath.Join("..", "..", "examples"), filepath.Join("..", "..", "bootstrap")} {
		if err := filepath.WalkDir(dir, addSource); err != nil {
			t.Fatal(err)
		}
	}
	fixtures := t.TempDir()
	for name, input := range map[string]string{
		"tokens": "import pub match fn let mut if else while for in break continue return struct true false\r\n" +
			"= => == ! != < <= > >= & && | || - -> + * / % ( ) { } [ ] , : :: . ; ?\n" +
			"let clément_٣ = ١_٢.٣_٤ // 文\r\n/* block\r\n */ \"escaped\\n\\t\\\"\\0\" \"💡\" \xff @\n",
		"strings": "\"escaped\\\r\nnewline\"\n\"unterminated\r\nfn next\n\"eof\\",
		"comment": "let x = 1 /* unclosed\r\n",
		"nul":     "let x\x00 ignored",
		"empty":   "",
	} {
		file := filepath.Join(fixtures, name+".gors")
		if err := os.WriteFile(file, []byte(input), 0600); err != nil {
			t.Fatal(err)
		}
		files = append(files, file)
	}

	for _, backend := range []compiler.Backend{compiler.BackendC, compiler.BackendGo} {
		t.Run(string(backend), func(t *testing.T) {
			bin := buildBootstrapTool(t, entry, backend)
			for _, path := range files {
				t.Run(filepath.ToSlash(path), func(t *testing.T) {
					checkBootstrapTokens(t, bin, path)
				})
			}
		})
	}
}

func buildBootstrapTool(t *testing.T, entry string, backend compiler.Backend) string {
	t.Helper()
	output, errs := compiler.CompileFile(entry, backend)
	if len(errs) != 0 {
		t.Fatalf("compile: %v", errs)
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "tool")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	src, tool := filepath.Join(dir, "main.go"), "go"
	args := []string{"build", "-o", bin, src}
	if backend == compiler.BackendC {
		src, tool = filepath.Join(dir, "main.c"), "gcc"
		args = []string{"-std=c23", "-O2", "-o", bin, src}
	}
	if err := os.WriteFile(src, []byte(output), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if out, err := exec.CommandContext(ctx, tool, args...).CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	return bin
}

func TestBootstrapImportParser(t *testing.T) {
	entry := filepath.Join("..", "..", "bootstrap", "parser", "main.gors")
	cases := []struct {
		name, input, output string
		status              int
	}{
		{"multiple", ";\nimport a \"x.gors\"\n\nimport b \"y\"\nfn main() {}", "2 1 9 10 12 18\n4 1 28 29 31 32\n", 0},
		{"no imports", "pub fn f() {}", "", 0},
		{"body belongs to next parser stage", "fn f() {}\nimport later \"x\"", "", 0},
		{"missing alias", "import", "1 7 expected import alias\n", 1},
		{"missing path", "import foo\n", "2 1 expected quoted import path\n", 1},
		{"missing quotes", "import foo bare", "1 12 expected quoted import path\n", 1},
		{"public import", "pub import a \"x\"", "1 5 an import cannot be public\n", 1},
	}
	dir := t.TempDir()
	for _, tc := range cases {
		if err := os.WriteFile(filepath.Join(dir, tc.name+".gors"), []byte(tc.input), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, backend := range []compiler.Backend{compiler.BackendC, compiler.BackendGo} {
		t.Run(string(backend), func(t *testing.T) {
			bin := buildBootstrapTool(t, entry, backend)
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					got := runBootstrapTool(t, bin, filepath.Join(dir, tc.name+".gors"), tc.status)
					if got != tc.output {
						t.Fatalf("got %q, want %q", got, tc.output)
					}
				})
			}
		})
	}
}

func TestBootstrapDeclarationParser(t *testing.T) {
	entry := filepath.Join("..", "..", "bootstrap", "parser", "headers.gors")
	dir := t.TempDir()
	valid := filepath.Join(dir, "declarations.gors")
	source := "import a \"a.gors\"\r\n" +
		"pub struct Point { x: a::Scalar, data: Option<[]int>, }\r\n" +
		"pub fn (p: &mut Point) change(x: Result<Option<int>, string>,) -> []int {\r\n" +
		"    if true { println(\"}\" /* { */) }; return [1]\r\n" +
		"}\r\nimport b \"b.gors\"; fn last() {}\r\n"
	if err := os.WriteFile(valid, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	cases := []struct{ name, source, output string }{
		{"missing type", "struct Point { x: }", "1 19 expected a type\n"},
		{"missing generic close", "fn f(x: Option<int) {}", "1 19 expected >\n"},
		{"unclosed body", "fn f() {\n    if true {}\n", "3 1 expected } to close function body\n"},
		{"bad top level", "pub let x = 3", "1 5 expected struct or fn declaration\n"},
		{"interleaved public import", "fn f() {}\npub import a \"x\"", "2 5 an import cannot be public\n"},
	}
	for _, tc := range cases {
		if err := os.WriteFile(filepath.Join(dir, tc.name+".gors"), []byte(tc.source), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, backend := range []compiler.Backend{compiler.BackendC, compiler.BackendGo} {
		t.Run(string(backend), func(t *testing.T) {
			bin := buildBootstrapTool(t, entry, backend)
			for _, path := range []string{valid, entry, filepath.Join(filepath.Dir(entry), "declarations.gors"),
				filepath.Join("..", "..", "examples", "modules", "math.gors")} {
				t.Run(filepath.Base(path), func(t *testing.T) {
					compareBootstrapDeclarations(t, bin, path)
				})
			}
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					got := runBootstrapTool(t, bin, filepath.Join(dir, tc.name+".gors"), 1)
					if got != tc.output {
						t.Fatalf("got %q, want %q", got, tc.output)
					}
				})
			}
		})
	}
}

func compareBootstrapDeclarations(t *testing.T, bin, path string) {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	program, errs := parser.Parse(string(src))
	if len(errs) != 0 {
		t.Fatalf("Go parser: %v", errs)
	}
	type header struct {
		pos        token.Pos
		kind, name string
		public     bool
		bindings   int
	}
	var want []header
	for _, d := range program.Structs {
		want = append(want, header{d.At, "struct", d.Name, d.Public, len(d.Fields)})
	}
	for _, d := range program.Funcs {
		want = append(want, header{d.At, "fn", d.Name, d.Public, len(d.Params)})
	}
	sort.Slice(want, func(i, j int) bool {
		a, b := want[i].pos, want[j].pos
		return a.Line < b.Line || (a.Line == b.Line && a.Col < b.Col)
	})
	out := runBootstrapTool(t, bin, path, 0)
	rows := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(rows) != len(want) {
		t.Fatalf("got %d declarations, want %d\n%s", len(rows), len(want), out)
	}
	for i, row := range rows {
		var line, col, start, end, bindings int
		var kind string
		var public bool
		if _, err := fmt.Sscan(row, &line, &col, &kind, &public, &start, &end, &bindings); err != nil ||
			start < 0 || end < start || end > len(src) {
			t.Fatalf("invalid header %q", row)
		}
		d := want[i]
		if line != d.pos.Line || col != d.pos.Col || kind != d.kind || public != d.public ||
			string(src[start:end]) != d.name || bindings != d.bindings {
			t.Fatalf("got %q, want %+v", row, d)
		}
	}
}

func TestBootstrapBodyParser(t *testing.T) {
	entry := filepath.Join("..", "..", "bootstrap", "parser", "tree.gors")
	cases := []struct{ name, input, tree string }{
		{"precedence and return", "fn f() { let mut total: int = 1 + 2 * 3 - 4; return total }",
			"(block (let_mut name type (binary:- (binary:+ INT (binary:* INT INT)) INT)) (return name))"},
		{"postfix and literals", "fn f() { let x = m::open().items[:2]?; let y = Pt { x: [1,2][0] }; }",
			"(block (let name - (try (slice (field (call name) name) - INT))) (let name - (struct_literal name (initializer name (index (slice_literal INT INT) INT)))))"},
		{"control flow", "fn f(){ for i,x in xs { if i == 0 {continue} else if let Some(v) = x { total = total + v } else {break} }; while !done { done = true } }",
			"(block (for name name name (block (if - (binary:== name INT) (block continue) (if_let (pattern name name) name (block (assign name (binary:+ name name))) (block break))))) (while (unary:! name) (block (assign name true))))"},
		{"match and references", "fn f(){ let x = match a { Some(v) => { trace(v); v }, None => 0 }; match x { Ok(v) => { use(&mut v) }, Err(e) => log(e) }; return; }",
			"(block (let name - (match_expression name (arm (pattern name name) (block (expr_stmt (call name name)) (expr_stmt name))) (arm (pattern name -) INT))) (match_statement name (arm (pattern name name) (block (expr_stmt (call name (unary:&mut name))))) (arm (pattern name name) (call name name))) return)"},
	}
	malformed := []struct{ name, input, diagnostic string }{
		{"missing initializer", "fn f() { let x = }", "1 18 expected expression\n"},
		{"unclosed call", "fn f() { f(1 }", "1 14 expected )\n"},
		{"missing loop keyword", "fn f() { for i xs {} }", "1 16 expected in\n"},
		{"missing match arrow", "fn f() { match x { Ok(v) v } }", "1 26 expected =>\n"},
	}
	dir := t.TempDir()
	writeFixture := func(name, input string) string {
		file := filepath.Join(dir, name+".gors")
		if err := os.WriteFile(file, []byte(input), 0600); err != nil {
			t.Fatal(err)
		}
		return file
	}
	validFiles, badFiles := make(map[string]string), make(map[string]string)
	for _, tc := range cases {
		validFiles[tc.name] = writeFixture(tc.name, tc.input)
	}
	for _, tc := range malformed {
		badFiles[tc.name] = writeFixture(tc.name, tc.input)
	}
	var corpus []string
	for _, name := range []string{"examples", "bootstrap"} {
		dir := filepath.Join("..", "..", name)
		err := filepath.WalkDir(dir, func(path string, item fs.DirEntry, err error) error {
			if err == nil && !item.IsDir() && filepath.Ext(path) == ".gors" {
				corpus = append(corpus, path)
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, backend := range []compiler.Backend{compiler.BackendC, compiler.BackendGo} {
		t.Run(string(backend), func(t *testing.T) {
			bin := buildBootstrapTool(t, entry, backend)
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					out := runBootstrapTool(t, bin, validFiles[tc.name], 0)
					got := readBootstrapTrees(t, out, len(lexer.New(tc.input).Tokens())-1)
					if len(got) != 1 || got[0] != tc.tree {
						t.Fatalf("got %v\nwant %s", got, tc.tree)
					}
				})
			}
			for _, tc := range malformed {
				t.Run(tc.name, func(t *testing.T) {
					got := runBootstrapTool(t, bin, badFiles[tc.name], 1)
					if got != tc.diagnostic {
						t.Fatalf("got %q, want %q", got, tc.diagnostic)
					}
				})
			}
			for _, path := range corpus {
				t.Run(filepath.ToSlash(path), func(t *testing.T) {
					src, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					program, errs := parser.Parse(string(src))
					if len(errs) != 0 {
						t.Fatalf("Go parser: %v", errs)
					}
					out := runBootstrapTool(t, bin, path, 0)
					got := readBootstrapTrees(t, out, len(lexer.New(string(src)).Tokens())-1)
					if len(got) != len(program.Funcs) {
						t.Fatalf("got %d body roots, want %d", len(got), len(program.Funcs))
					}
				})
			}
		})
	}
}

func readBootstrapTrees(t *testing.T, output string, tokenCount int) []string {
	t.Helper()
	type node struct {
		first, next int
		text        string
	}
	var nodes []node
	var roots []string
	for _, row := range strings.Split(strings.TrimSpace(output), "\n") {
		fields := strings.Fields(row)
		if len(fields) == 0 {
			continue
		}
		if fields[0] == "root" {
			var id int
			if len(fields) != 2 {
				t.Fatalf("invalid root %q", row)
			}
			if _, err := fmt.Sscan(fields[1], &id); err != nil || id < 0 || id >= len(nodes) ||
				!(nodes[id].text == "block" || strings.HasPrefix(nodes[id].text, "(block ")) {
				t.Fatalf("invalid body root %q", row)
			}
			roots = append(roots, nodes[id].text)
			continue
		}
		var first, next, id int
		var kind string
		if _, err := fmt.Sscan(row, &id, &kind, &first, &next); err != nil ||
			id != len(nodes) || first < 0 || first >= next || next > tokenCount {
			t.Fatalf("invalid node %q", row)
		}
		text := kind
		if len(fields) > 4 {
			parts := []string{kind}
			for _, field := range fields[4:] {
				child, err := strconv.Atoi(field)
				if err != nil || child < -1 || child >= id {
					t.Fatalf("invalid child in %q", row)
				}
				if child == -1 {
					parts = append(parts, "-")
				} else {
					n := nodes[child]
					if n.first < first || n.next > next {
						t.Fatalf("child span escapes parent in %q", row)
					}
					parts = append(parts, n.text)
				}
			}
			text = "(" + strings.Join(parts, " ") + ")"
		}
		nodes = append(nodes, node{first, next, text})
	}
	return roots
}

func TestBootstrapSemanticChecker(t *testing.T) {
	entry := filepath.Join("..", "..", "bootstrap", "sema", "main.gors")
	dir := t.TempDir()
	cases := []struct{ name, source, diagnostic string }{
		{"forward call and scopes", `fn main(args: []string) -> int {
			let mut total = 0
			let empty: []int = []
			let values = [1, 2, 3]
			for i, v in values[1:] {
				if i == 0 { let label = "item"; println(label) }
				total = total + v
			}
			{ let label = "done"; println(label) }
			return add(total, len(args) + len(empty))
		}
		fn add(a: int, b: int) -> int { return a + b }`, ""},
		{"all paths return", `fn select(ok: bool) -> string {
			if ok { return "yes" } else { return "no" }
		}
		fn main(){ println(select(1 < 2 && !false)) }`, ""},
		{"unknown local", "fn main() { println(missing) }", "1 21 unknown local name\n"},
		{"scope escape", "fn main() { { let x = 1 }; println(x) }", "unknown local name"},
		{"duplicate local", "fn main() { let x = 1; let x = 2 }", "name already declared"},
		{"duplicate parameter", "fn f(a: int, a: int) {} fn main() {}", "name already declared"},
		{"unknown function", "fn main() { unknown() }", "unknown function"},
		{"bad arity", "fn f(x: int) {} fn main() { f() }", "wrong number of arguments"},
		{"argument type", "fn f(x: int) {} fn main() { f(true) }", "argument type mismatch"},
		{"initializer type", "fn main() { let x: int = true }", "initializer type mismatch"},
		{"immutable assignment", "fn main() { let x = 1; x = 2 }", "assignment to immutable name"},
		{"bad condition", "fn main() { while 1 {} }", "while condition must be bool"},
		{"bad return", "fn f() -> int { return true } fn main() {}", "return type mismatch"},
		{"missing return", "fn f(x: bool) -> int { if x { return 1 } } fn main() {}", "missing a return"},
		{"loop control", "fn main() { break }", "loop control outside a loop"},
		{"bad index", "fn main() { let xs = [1]; println(xs[false]) }", "index or bound must be int"},
		{"mixed slice", "fn main() { let xs = [1, true] }", "slice element type mismatch"},
		{"unsupported local mutable reference", "struct Box { value: int } fn main() { let mut b = Box { value: 1 }; let r = &mut b }", "local mutable reference bindings are outside"},
		{"missing import", `import a "a.gors" fn main() {}`, "could not read module"},
	}
	for _, tc := range cases {
		path := filepath.Join(dir, tc.name+".gors")
		if err := os.WriteFile(path, []byte(tc.source), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, backend := range []compiler.Backend{compiler.BackendC, compiler.BackendGo} {
		t.Run(string(backend), func(t *testing.T) {
			bin := buildBootstrapTool(t, entry, backend)
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					path := filepath.Join(dir, tc.name+".gors")
					if tc.diagnostic == "" {
						if _, errs := compiler.CompileFile(path, backend); len(errs) != 0 {
							t.Fatalf("existing compiler rejected checker fixture: %v", errs)
						}
						if got := runBootstrapTool(t, bin, path, 0); got != "" {
							t.Fatalf("unexpected output %q", got)
						}
						return
					}
					got := runBootstrapTool(t, bin, path, 1)
					if !strings.Contains(got, tc.diagnostic) || strings.Contains(got, "leaked allocation") {
						t.Fatalf("got %q, want diagnostic containing %q", got, tc.diagnostic)
					}
				})
			}
			for _, name := range []string{"hello", "fib", "loop", "for_loop", "break_continue", "subslice"} {
				t.Run(name, func(t *testing.T) {
					path := filepath.Join("..", "..", "examples", name+".gors")
					if got := runBootstrapTool(t, bin, path, 0); got != "" {
						t.Fatalf("unexpected output %q", got)
					}
				})
			}
		})
	}
}

func checkBootstrapTokens(t *testing.T, bin, path string) {
	t.Helper()
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := lexer.New(string(source)).Tokens()
	wantExit := 0
	for _, tok := range want {
		if tok.Type == token.ILLEGAL {
			wantExit = 1
			break
		}
	}
	out := runBootstrapTool(t, bin, path, wantExit)
	rows := strings.Split(strings.TrimSpace(out), "\n")
	if len(rows) != len(want) {
		t.Fatalf("got %d tokens, want %d\n%s", len(rows), len(want), out)
	}
	// Map normalized rune positions back to offsets in the original file.
	offsets := make(map[[2]int]int)
	line, col := 1, 1
	for pos := 0; ; {
		offsets[[2]int{line, col}] = pos
		if pos == len(source) {
			break
		}
		r, width := utf8.DecodeRune(source[pos:])
		if r == '\r' && pos+1 < len(source) && source[pos+1] == '\n' {
			r, width = '\n', 2
		}
		pos += width
		col++
		if r == '\n' {
			line, col = line+1, 1
		}
	}
	lastEnd := 0
	for i, row := range rows {
		var line, col, start, end int
		var kind string
		var extra string
		n, _ := fmt.Sscan(row, &line, &col, &kind, &start, &end, &extra)
		tok := want[i]
		if n != 5 || line != tok.Pos.Line || col != tok.Pos.Col || kind != string(tok.Type) {
			t.Fatalf("token %d: got %q, want %d %d %s", i, row, tok.Pos.Line, tok.Pos.Col, tok.Type)
		}
		if start != offsets[[2]int{line, col}] || start < lastEnd || end < start || end > len(source) {
			t.Fatalf("token %d: invalid byte span in %q", i, row)
		}
		raw := string(source[start:end])
		switch tok.Type {
		case token.INT, token.FLOAT:
			if strings.ReplaceAll(raw, "_", "") != tok.Lit {
				t.Fatalf("token %d: raw number %q, want %q", i, raw, tok.Lit)
			}
		case token.STRING:
			if len(raw) < 2 || raw[0] != '"' || raw[len(raw)-1] != '"' {
				t.Fatalf("token %d: invalid quoted span %q", i, raw)
			}
		case token.ILLEGAL:
			if start == end {
				t.Fatalf("token %d: empty span for an invalid token", i)
			}
		default:
			if raw != tok.Lit {
				t.Fatalf("token %d: raw text %q, want %q", i, raw, tok.Lit)
			}
		}
		lastEnd = end
	}
}

func runBootstrapTool(t *testing.T, bin, path string, wantExit int) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, path)
	cmd.Env = append(os.Environ(), "GORS_LEAKCHECK=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		var exit *exec.ExitError
		if wantExit == 0 || !errors.As(err, &exit) || exit.ExitCode() != wantExit {
			t.Fatalf("run: %v\n%s", err, out)
		}
	} else if wantExit != 0 {
		t.Fatalf("expected exit status %d", wantExit)
	}
	return strings.ReplaceAll(string(out), "\r\n", "\n")
}
