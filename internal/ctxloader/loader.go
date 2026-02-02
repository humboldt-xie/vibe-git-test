package ctxloader

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// FileReference represents a file referenced in an issue
type FileReference struct {
	Path    string
	Content string
	Found   bool
}

// ExtractFileReferences extracts @ mentions from text
// Supports formats: @filename, @path/to/file, @"file with spaces"
func ExtractFileReferences(text string) []string {
	var refs []string

	// Pattern: @"file with spaces" or @filename or @path/to/file
	// Capture quoted strings or unquoted path-like strings
	patterns := []string{
		`@"([^"]+)"`,           // @"file with spaces"
		`@([a-zA-Z0-9_./-]+)`, // @filename or @path/to/file
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindAllStringSubmatch(text, -1)
		for _, match := range matches {
			if len(match) > 1 {
				path := strings.TrimSpace(match[1])
				if path != "" && !contains(refs, path) {
					refs = append(refs, path)
				}
			}
		}
	}

	return refs
}

// LoadReferencedFiles loads the content of referenced files
func LoadReferencedFiles(refs []string, repoRoot string) []*FileReference {
	var files []*FileReference

	for _, ref := range refs {
		file := &FileReference{
			Path: ref,
		}

		// Try different path resolutions
		pathsToTry := []string{
			filepath.Join(repoRoot, ref),
			ref,
			filepath.Join(repoRoot, "src", ref),
			filepath.Join(repoRoot, "pkg", ref),
		}

		for _, path := range pathsToTry {
			content, err := os.ReadFile(path)
			if err == nil {
				file.Content = string(content)
				file.Found = true
				break
			}
		}

		files = append(files, file)
	}

	return files
}

// BuildReferencedFilesSection builds the prompt section for referenced files
func BuildReferencedFilesSection(files []*FileReference) string {
	if len(files) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n## Referenced Files (from issue @mentions)\n\n")

	for _, f := range files {
		if f.Found {
			sb.WriteString(fmt.Sprintf("### %s\n```\n%s\n```\n\n", f.Path, f.Content))
		} else {
			sb.WriteString(fmt.Sprintf("### %s\n**File not found**\n\n", f.Path))
		}
	}

	return sb.String()
}

// BuildCodebaseSection builds the codebase context section
func BuildCodebaseSection(root string, excludeFiles []string) (string, error) {
	var result strings.Builder

	excludeMap := make(map[string]bool)
	for _, f := range excludeFiles {
		excludeMap[f] = true
	}

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			name := info.Name()
			if strings.HasPrefix(name, ".") || name == "vendor" || name == "node_modules" ||
				name == "dist" || name == "build" || name == ".git" {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip if in referenced files (will be shown separately)
		if excludeMap[path] {
			return nil
		}

		// Skip certain file types
		ext := filepath.Ext(path)
		if ext == ".exe" || ext == ".dll" || ext == ".so" || ext == ".dylib" ||
			ext == ".bin" || ext == ".log" || ext == ".tmp" {
			return nil
		}

		// Skip large files
		if info.Size() > 100*1024 {
			result.WriteString(fmt.Sprintf("\n// File: %s (skipped - too large)\n", path))
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		result.WriteString(fmt.Sprintf("\n// File: %s\n", path))
		result.WriteString(string(content))
		result.WriteString("\n")

		return nil
	})

	if err != nil {
		return "", err
	}

	return result.String(), nil
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
