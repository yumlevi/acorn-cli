package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Version is the running binary's release tag — set by main.go from
// the `var version` constant (which is overrideable via -ldflags
// "-X main.version=vX.Y.Z" so CI can stamp it). Compared against the
// GitHub `latest` tag to tell the user whether they're up to date.
var Version = "dev"

// SetVersion is the wiring hook for cmd/acorn/main.go.
func SetVersion(v string) { Version = v }

// versionLE returns true when 'a' is older than or equal to 'b' under
// loose semver: 'v1.2.3' parts compared numerically, missing parts as
// zero, leading 'v' optional. Returns false if either string is
// unparseable so we lean toward 'show update available' rather than
// silently skip.
func versionLE(a, b string) bool {
	pa, oka := parseSemver(a)
	pb, okb := parseSemver(b)
	if !oka || !okb {
		return false
	}
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			return pa[i] < pb[i]
		}
	}
	return true
}

func parseSemver(s string) ([3]int, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	parts := strings.SplitN(s, "-", 2)[0] // drop pre-release suffix
	xs := strings.Split(parts, ".")
	var out [3]int
	for i := 0; i < 3 && i < len(xs); i++ {
		n := 0
		for _, c := range xs[i] {
			if c < '0' || c > '9' {
				return out, false
			}
			n = n*10 + int(c-'0')
		}
		out[i] = n
	}
	return out, true
}

// updateCheckResult carries GitHub release info back to the UI.
type updateCheckResult struct {
	OK      bool
	Version string
	URL     string
	Err     string
}

func (r updateCheckResult) teaMsg() tea.Msg { return r }

// checkUpdateCmd pings GitHub for the latest release tag.
func checkUpdateCmd(checkOnly bool) tea.Cmd {
	_ = checkOnly // no distinction for now — we never install in-process.
	return func() tea.Msg {
		client := &http.Client{Timeout: 8 * time.Second}
		req, _ := http.NewRequest("GET", "https://api.github.com/repos/yumlevi/acorn-cli/releases/latest", nil)
		req.Header.Set("Accept", "application/vnd.github+json")
		resp, err := client.Do(req)
		if err != nil {
			return updateCheckResult{Err: err.Error()}
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != 200 {
			return updateCheckResult{Err: fmt.Sprintf("HTTP %d", resp.StatusCode)}
		}
		var rel struct {
			TagName string `json:"tag_name"`
			URL     string `json:"html_url"`
		}
		if err := json.Unmarshal(body, &rel); err != nil {
			return updateCheckResult{Err: err.Error()}
		}
		return updateCheckResult{OK: true, Version: rel.TagName, URL: rel.URL}
	}
}
