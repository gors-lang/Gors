package cgen

import (
	_ "embed"
	"fmt"
	"strconv"
	"strings"
)

//go:embed native_runtime.gors
var nativeRuntime string

// NativeRuntime extracts the shared C source from the GoRs string array.
func NativeRuntime() (string, error) {
	source := strings.ReplaceAll(nativeRuntime, "\r\n", "\n")
	_, body, ok := strings.Cut(source, "    return [\n")
	if !ok {
		return "", fmt.Errorf("native runtime is missing its C source array")
	}
	lines, _, ok := strings.Cut(body, "\n    ]")
	if !ok {
		return "", fmt.Errorf("native runtime has an unclosed C source array")
	}
	var output strings.Builder
	for _, line := range strings.Split(lines, "\n") {
		value, err := strconv.Unquote(strings.TrimSuffix(strings.TrimSpace(line), ","))
		if err != nil {
			return "", fmt.Errorf("decode native runtime line: %w", err)
		}
		output.WriteString(value)
		output.WriteByte('\n')
	}
	return output.String(), nil
}
