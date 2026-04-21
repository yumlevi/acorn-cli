package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

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
