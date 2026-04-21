package tools

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var noiseDirs = map[string]bool{
	".git": true, "node_modules": true, "__pycache__": true, ".venv": true, "venv": true,
	"dist": true, "build": true, ".next": true, ".cache": true, "target": true,
}

// Glob implements the glob tool. Input: pattern, path.
func Glob(input map[string]any, cwd string) any {
	pattern := asString(input["pattern"], "*")
	searchPath := asString(input["path"], cwd)
	if !filepath.IsAbs(searchPath) {
		searchPath = filepath.Join(cwd, searchPath)
	}

	var matches []string
	err := filepath.WalkDir(searchPath, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if strings.HasPrefix(name, ".") || noiseDirs[name] {
				return fs.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(searchPath, path)
		if match, _ := filepath.Match(pattern, rel); match {
			matches = append(matches, rel)
		} else if match, _ := filepath.Match(pattern, name); match {
			matches = append(matches, rel)
		}
		if len(matches) >= 500 {
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return map[string]string{"error": err.Error()}
	}
	if len(matches) > 500 {
		matches = matches[:500]
	}
	return map[string]any{"matches": matches, "count": len(matches)}
}

// Grep implements the grep tool. Input: pattern, path, glob/type, -i.
func Grep(input map[string]any, cwd string) any {
	pattern := asString(input["pattern"], "")
	if pattern == "" {
		return map[string]string{"error": "pattern is required"}
	}
	searchPath := asString(input["path"], cwd)
	if !filepath.IsAbs(searchPath) {
		searchPath = filepath.Join(cwd, searchPath)
	}
	fileGlob := asString(input["glob"], asString(input["type"], ""))

	pre := ""
	if asBool(input["-i"], false) {
		pre = "(?i)"
	}
	re, err := regexp.Compile(pre + pattern)
	if err != nil {
		return map[string]string{"error": "Invalid regex: " + err.Error()}
	}

	type hit struct {
		File string `json:"file"`
		Line int    `json:"line"`
		Text string `json:"text"`
	}
	var results []hit
	truncated := false

	err = filepath.WalkDir(searchPath, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if strings.HasPrefix(name, ".") || noiseDirs[name] {
				return fs.SkipDir
			}
			return nil
		}
		if fileGlob != "" {
			if match, _ := filepath.Match(fileGlob, d.Name()); !match {
				return nil
			}
		}
		rel, _ := filepath.Rel(searchPath, path)
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 64<<10), 4<<20)
		lineNo := 0
		for sc.Scan() {
			lineNo++
			line := sc.Text()
			if re.MatchString(line) {
				if len(line) > 200 {
					line = line[:200]
				}
				results = append(results, hit{File: rel, Line: lineNo, Text: line})
				if len(results) >= 200 {
					truncated = true
					return filepath.SkipAll
				}
			}
		}
		return nil
	})
	if err != nil && !truncated {
		return map[string]string{"error": err.Error()}
	}
	out := map[string]any{"results": results, "count": len(results)}
	if truncated {
		out["truncated"] = true
	}
	return out
}
