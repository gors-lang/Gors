package codegen

const nativeRuntime = `
func __gors_native_process(name string, args ...string) int64 {
    cmd := exec.Command(name, args...)
    cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
    if err := cmd.Run(); err != nil {
        if child, ok := err.(*exec.ExitError); ok {
            if code := child.ExitCode(); code >= 0 {
                return int64(code & 255)
            }
            return 1
        }
        fmt.Fprintln(os.Stderr, "gors-bootstrap:", err)
        return 127
    }
    return 0
}

func __gors_native_c(source, output string, args []string) int64 {
    parent := ""
    if output != "" {
        parent = filepath.Dir(output)
    }
    dir, err := os.MkdirTemp(parent, ".gors-*")
    if err != nil {
        fmt.Fprintln(os.Stderr, "gors-bootstrap: could not create a temporary directory")
        return 2
    }
    defer os.RemoveAll(dir)
    input := filepath.Join(dir, "main.c")
    binary := filepath.Join(dir, "program")
    if runtime.GOOS == "windows" {
        binary += ".exe"
    }
    if err := os.WriteFile(input, []byte(source), 0600); err != nil {
        fmt.Fprintln(os.Stderr, "gors-bootstrap: could not write temporary C source")
        return 2
    }
    if status := __gors_native_process("gcc", "-std=c23", "-O2", input, "-o", binary); status != 0 {
        return status
    }
    if output == "" {
        return __gors_native_process(binary, args...)
    }
    if err := os.Rename(binary, output); err != nil {
        fmt.Fprintln(os.Stderr, "gors-bootstrap: could not replace the output executable")
        return 2
    }
    return 0
}
`
