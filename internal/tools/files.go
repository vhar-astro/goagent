package tools

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
)

const (
	ReadFileToolName  = "read_file"
	WriteFileToolName = "write_file"
)

var (
	ErrFileNotFound   = errors.New("file does not exist")
	ErrFileIsDir      = errors.New("path must reference a file")
	ErrFileContentNil = errors.New("content is required")
)

// FileRuntime captures the workspace-boundary helpers required by file tools.
type FileRuntime interface {
	WorkspaceRoot() string
	ResolvePath(path string) (string, error)
}

// ReadFileRequest describes one workspace-scoped file read.
type ReadFileRequest struct {
	Path string
}

// ReadFileResult contains the normalized file-read payload for later executor code.
type ReadFileResult struct {
	Path          string
	ResolvedPath  string
	Content       string
	OriginalBytes int
	Truncated     bool
	DroppedBytes  int
}

// WriteFileRequest describes one workspace-scoped file write.
type WriteFileRequest struct {
	Path          string
	Content       *string
	CreateParents bool
	FileMode      os.FileMode
}

// WriteFileResult contains the normalized file-write result for later executor code.
type WriteFileResult struct {
	Path         string
	ResolvedPath string
	Created      bool
	BytesWritten int
}

// FileToolSpecs returns the built-in tool schemas for workspace-scoped file IO.
func FileToolSpecs() []Spec {
	return []Spec{
		{
			Name:        ReadFileToolName,
			Description: "Read a text file inside the current workspace.",
			Capability:  CapabilityRead,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "Relative or absolute file path inside the workspace.",
					},
				},
				"required":             []string{"path"},
				"additionalProperties": false,
			},
		},
		{
			Name:        WriteFileToolName,
			Description: "Create or replace a text file inside the current workspace.",
			Capability:  CapabilityWrite,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "Relative or absolute file path inside the workspace.",
					},
					"content": map[string]any{
						"type":        "string",
						"description": "Full replacement file content.",
					},
					"create_parents": map[string]any{
						"type":        "boolean",
						"description": "Create missing parent directories before writing.",
					},
				},
				"required":             []string{"path", "content"},
				"additionalProperties": false,
			},
		},
	}
}

// ReadFile reads one file after enforcing the workspace boundary and tool-output limit.
func ReadFile(runtime FileRuntime, req ReadFileRequest) (ReadFileResult, error) {
	resolvedPath, err := runtime.ResolvePath(req.Path)
	if err != nil {
		return ReadFileResult{}, err
	}

	info, err := os.Stat(resolvedPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ReadFileResult{}, fmt.Errorf("read file %q: %w", resolvedPath, ErrFileNotFound)
		}
		return ReadFileResult{}, fmt.Errorf("stat file %q: %w", resolvedPath, err)
	}
	if info.IsDir() {
		return ReadFileResult{}, fmt.Errorf("read file %q: %w", resolvedPath, ErrFileIsDir)
	}

	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		return ReadFileResult{}, fmt.Errorf("read file %q: %w", resolvedPath, err)
	}

	content, originalBytes, truncated, droppedBytes := limitToolOutput(runtime, string(data))
	return ReadFileResult{
		Path:          displayPath(runtime, resolvedPath),
		ResolvedPath:  resolvedPath,
		Content:       content,
		OriginalBytes: originalBytes,
		Truncated:     truncated,
		DroppedBytes:  droppedBytes,
	}, nil
}

// WriteFile creates or replaces one file after enforcing the workspace boundary.
func WriteFile(runtime FileRuntime, req WriteFileRequest) (WriteFileResult, error) {
	if req.Content == nil {
		return WriteFileResult{}, ErrFileContentNil
	}

	resolvedPath, err := runtime.ResolvePath(req.Path)
	if err != nil {
		return WriteFileResult{}, err
	}

	existingInfo, err := os.Stat(resolvedPath)
	if err == nil && existingInfo.IsDir() {
		return WriteFileResult{}, fmt.Errorf("write file %q: %w", resolvedPath, ErrFileIsDir)
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return WriteFileResult{}, fmt.Errorf("stat file %q: %w", resolvedPath, err)
	}

	if req.CreateParents {
		parentDir := filepath.Dir(resolvedPath)
		if mkdirErr := os.MkdirAll(parentDir, 0o755); mkdirErr != nil {
			return WriteFileResult{}, fmt.Errorf("create parent directories for %q: %w", resolvedPath, mkdirErr)
		}
	}

	mode := req.FileMode
	if mode == 0 {
		mode = 0o644
	}

	if writeErr := os.WriteFile(resolvedPath, []byte(*req.Content), mode); writeErr != nil {
		return WriteFileResult{}, fmt.Errorf("write file %q: %w", resolvedPath, writeErr)
	}

	created := errors.Is(err, os.ErrNotExist)
	return WriteFileResult{
		Path:         displayPath(runtime, resolvedPath),
		ResolvedPath: resolvedPath,
		Created:      created,
		BytesWritten: len(*req.Content),
	}, nil
}

func displayPath(runtime FileRuntime, resolvedPath string) string {
	relativePath, err := filepath.Rel(runtime.WorkspaceRoot(), resolvedPath)
	if err != nil || relativePath == "." {
		return resolvedPath
	}

	return filepath.Clean(relativePath)
}

func limitToolOutput(runtime any, text string) (content string, originalBytes int, truncated bool, droppedBytes int) {
	content = text
	originalBytes = len(text)

	method := reflect.ValueOf(runtime).MethodByName("LimitToolOutput")
	if !method.IsValid() {
		return content, originalBytes, false, 0
	}

	results := method.Call([]reflect.Value{reflect.ValueOf(text)})
	if len(results) != 1 {
		return content, originalBytes, false, 0
	}

	result := results[0]
	if result.Kind() == reflect.Pointer && result.IsNil() {
		return content, originalBytes, false, 0
	}
	if result.Kind() == reflect.Pointer {
		result = result.Elem()
	}
	if result.Kind() != reflect.Struct {
		return content, originalBytes, false, 0
	}

	if field := result.FieldByName("Text"); field.IsValid() && field.Kind() == reflect.String {
		content = field.String()
	}
	if field := result.FieldByName("OriginalBytes"); field.IsValid() && field.Kind() == reflect.Int {
		originalBytes = int(field.Int())
	}
	if field := result.FieldByName("Truncated"); field.IsValid() && field.Kind() == reflect.Bool {
		truncated = field.Bool()
	}

	method = result.MethodByName("DroppedBytes")
	if method.IsValid() {
		values := method.Call(nil)
		if len(values) == 1 && values[0].Kind() == reflect.Int {
			droppedBytes = int(values[0].Int())
		}
	}

	return content, originalBytes, truncated, droppedBytes
}
