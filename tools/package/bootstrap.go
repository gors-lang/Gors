package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"gors/internal/compiler"
)

func buildBootstrapSeed(work, cc string) ([]byte, error) {
	const entry = "bootstrap/cgen/main.gors"
	compilerPath, err := exec.LookPath(cc)
	if err != nil {
		return nil, fmt.Errorf("find C23 compiler %q: %w", cc, err)
	}
	seed, errs := compiler.CompileFile(entry, compiler.BackendC)
	if len(errs) != 0 {
		return nil, fmt.Errorf("generate initial bootstrap C: %v", errs)
	}
	stage1C := filepath.Join(work, "stage1.c")
	stage2C := filepath.Join(work, "stage2.c")
	stage3C := filepath.Join(work, "stage3.c")
	stage1 := filepath.Join(work, "stage1")
	stage2 := filepath.Join(work, "stage2")
	if runtime.GOOS == "windows" {
		stage1 += ".exe"
		stage2 += ".exe"
	}
	if err := os.WriteFile(stage1C, []byte(seed), 0600); err != nil {
		return nil, err
	}
	if err := seedCommand("build stage 1", false, compilerPath, "-std=c23", "-O2", stage1C, "-o", stage1); err != nil {
		return nil, err
	}
	if err := seedCommand("emit stage 2", true, stage1, "emit", entry, "-o", stage2C); err != nil {
		return nil, err
	}
	if err := seedCommand("build stage 2", false, compilerPath, "-std=c23", "-pedantic-errors", "-O2", stage2C, "-o", stage2); err != nil {
		return nil, err
	}
	if err := seedCommand("emit stage 3", true, stage2, "emit", entry, "-o", stage3C); err != nil {
		return nil, err
	}
	stage2Source, err := os.ReadFile(stage2C)
	if err != nil {
		return nil, err
	}
	stage3Source, err := os.ReadFile(stage3C)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(stage2Source, stage3Source) {
		return nil, fmt.Errorf("bootstrap verification failed: stage 2 and stage 3 emitted different C")
	}
	return stage2Source, nil
}

func seedCommand(step string, emptyPath bool, tool string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, tool, args...)
	if emptyPath {
		cmd.Env = append(os.Environ(), "PATH=")
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		return fmt.Errorf("%s: %w\n%s", step, err, out)
	}
	return nil
}
