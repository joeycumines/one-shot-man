package os

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
)

func scopedPathWithinRoot(root, target string) bool {
	relative, err := filepath.Rel(root, target)
	if err != nil || filepath.IsAbs(relative) {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func ensureScopedParent(root *os.Root, relative string, createDirs bool) (string, error) {
	parts := strings.Split(relative, string(filepath.Separator))
	current := ""
	for index := 0; index < len(parts)-1; index++ {
		if current == "" {
			current = parts[index]
		} else {
			current = filepath.Join(current, parts[index])
		}
		info, err := root.Lstat(current)
		if os.IsNotExist(err) {
			if !createDirs {
				return "", fmt.Errorf("parent directory does not exist")
			}
			if err := root.Mkdir(current, 0755); err != nil && !os.IsExist(err) {
				return "", fmt.Errorf("create parent directory: %w", err)
			}
			info, err = root.Lstat(current)
		}
		if err != nil {
			return "", fmt.Errorf("inspect parent directory: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("parent directory is a symlink")
		}
		if !info.IsDir() {
			return "", fmt.Errorf("parent path is not a directory")
		}
	}
	if current == "" {
		return ".", nil
	}
	return current, nil
}

func createScopedTemp(root *os.Root, parent string) (*os.File, string, error) {
	for attempt := 0; attempt < 10; attempt++ {
		var random [16]byte
		if _, err := rand.Read(random[:]); err != nil {
			return nil, "", fmt.Errorf("create temporary file name: %w", err)
		}
		name := filepath.Join(parent, ".osm-scoped-"+hex.EncodeToString(random[:])+".tmp")
		file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if os.IsExist(err) {
			continue
		}
		if err != nil {
			return nil, "", fmt.Errorf("create temporary file: %w", err)
		}
		return file, name, nil
	}
	return nil, "", fmt.Errorf("create temporary file: exhausted name attempts")
}

// writeFileScoped writes a relative path while keeping it inside root. It
// uses os.Root for the complete write-and-rename operation, so symlink and
// junction traversal cannot redirect the write outside the root. Replacing the
// destination with a same-directory temporary file prevents hard-linked
// destinations from being truncated in place; the platform owns the final
// rename's atomicity and durability characteristics.
func writeFileScoped(ctx context.Context, rootPath, relative, content string, mode os.FileMode, createDirs bool) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(rootPath) == "" {
		return fmt.Errorf("root path is required")
	}
	if strings.TrimSpace(relative) == "" {
		return fmt.Errorf("relative path is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	normalized := strings.ReplaceAll(relative, "\\", "/")
	relativePath := filepath.FromSlash(normalized)
	if filepath.IsAbs(relativePath) || filepath.VolumeName(relativePath) != "" {
		return fmt.Errorf("relative path must not be absolute")
	}
	cleanRelative := filepath.Clean(relativePath)
	if cleanRelative == "." || cleanRelative == ".." || strings.HasPrefix(cleanRelative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("relative path escapes root")
	}

	rootAbs, err := filepath.Abs(rootPath)
	if err != nil {
		return fmt.Errorf("resolve root: %w", err)
	}
	rootReal, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return fmt.Errorf("resolve root: %w", err)
	}
	rootInfo, err := os.Stat(rootReal)
	if err != nil {
		return fmt.Errorf("inspect root: %w", err)
	}
	if !rootInfo.IsDir() {
		return fmt.Errorf("root is not a directory")
	}
	root, err := os.OpenRoot(rootReal)
	if err != nil {
		return fmt.Errorf("open root: %w", err)
	}
	defer root.Close()

	target := filepath.Join(rootReal, cleanRelative)
	if !scopedPathWithinRoot(rootReal, target) {
		return fmt.Errorf("relative path escapes root")
	}
	parent, err := ensureScopedParent(root, cleanRelative, createDirs)
	if err != nil {
		return err
	}

	targetInfo, statErr := root.Lstat(cleanRelative)
	if statErr == nil {
		if targetInfo.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("target is a symlink")
		}
		if targetInfo.IsDir() {
			return fmt.Errorf("target is a directory")
		}
	} else if !os.IsNotExist(statErr) {
		return fmt.Errorf("inspect target: %w", statErr)
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	file, tempName, err := createScopedTemp(root, parent)
	if err != nil {
		return err
	}
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = root.Remove(tempName)
		}
	}()
	if err := file.Chmod(mode.Perm()); err != nil {
		_ = file.Close()
		return fmt.Errorf("set target permissions: %w", err)
	}
	if _, err := file.WriteString(content); err != nil {
		_ = file.Close()
		return fmt.Errorf("write target: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync target: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close target: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	if err := root.Rename(tempName, cleanRelative); err != nil {
		return fmt.Errorf("replace target: %w", err)
	}
	removeTemp = false
	return nil
}

func parseScopedWriteArgs(runtime *goja.Runtime, call goja.FunctionCall) (string, string, string, os.FileMode, bool) {
	var root, path, content string
	mode := os.FileMode(0644)
	createDirs := false
	if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
		root = call.Argument(0).String()
	}
	if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) && !goja.IsNull(call.Argument(1)) {
		path = call.Argument(1).String()
	}
	if len(call.Arguments) > 2 && !goja.IsUndefined(call.Argument(2)) && !goja.IsNull(call.Argument(2)) {
		content = call.Argument(2).String()
	}
	if len(call.Arguments) > 3 && !goja.IsUndefined(call.Argument(3)) && !goja.IsNull(call.Argument(3)) {
		opts := call.Argument(3).ToObject(runtime)
		if value := opts.Get("mode"); value != nil && !goja.IsUndefined(value) && !goja.IsNull(value) {
			mode = os.FileMode(value.ToInteger())
		}
		if value := opts.Get("createDirs"); value != nil && !goja.IsUndefined(value) && !goja.IsNull(value) {
			createDirs = value.ToBoolean()
		}
	}
	return root, path, content, mode, createDirs
}

func writeFileScopedBinding(ctx context.Context, adapter *gojaeventloop.Adapter, runtime *goja.Runtime, call goja.FunctionCall) goja.Value {
	if adapter == nil {
		panic(runtime.NewGoError(fmt.Errorf("writeFileScoped: event loop adapter is required")))
	}
	root, path, content, mode, createDirs := parseScopedWriteArgs(runtime, call)
	return adapter.TrackPromise(ctx, func(workerCtx context.Context, settle gojaeventloop.TrackedSettlement) {
		err := writeFileScoped(workerCtx, root, path, content, mode, createDirs)
		if err != nil {
			_ = settle.Settle(true, func(owner *goja.Runtime) any {
				return owner.NewGoError(fmt.Errorf("writeFileScoped: %w", err))
			})
			return
		}
		_ = settle.Settle(false, func(*goja.Runtime) any { return goja.Undefined() })
	})
}
