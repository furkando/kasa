package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// claudeBackend talks to the locally installed `claude` CLI in headless
// print mode, so a Claude Pro/Max subscription covers the usage — no API
// credits needed. Conversation continuity via --resume <session-id>.
type claudeBackend struct {
	system    string
	model     string // optional; empty = user's default
	sessionID string
}

func (c *claudeBackend) setSystem(system string) { c.system = system }

// cliLine is one line of `--output-format stream-json` output.
type cliLine struct {
	Type      string `json:"type"`
	SessionID string `json:"session_id"`
	IsError   bool   `json:"is_error"`
	Result    string `json:"result"`
	Event     *struct {
		Type  string `json:"type"`
		Delta *struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"delta"`
	} `json:"event"`
	Message *struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"message"`
}

func (c *claudeBackend) ask(ctx context.Context, question string, out io.Writer) error {
	args := []string{
		"-p", "--output-format", "stream-json", "--verbose",
		"--include-partial-messages", "--disallowed-tools", "*",
	}
	if c.model != "" {
		args = append(args, "--model", c.model)
	}
	if c.sessionID != "" {
		args = append(args, "--resume", c.sessionID)
	} else {
		args = append(args, "--append-system-prompt", c.system)
	}
	args = append(args, question)

	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Dir = chatDir() // fixed empty dir: no project CLAUDE.md/MCP noise, stable session store
	// The claude backend means "use the Pro/Max subscription login". A stray
	// ANTHROPIC_API_KEY in the shell would silently override it (and may point
	// at a credit-less org), so strip key vars from the child env.
	cmd.Env = envWithoutAPIKeys()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("claude cli başlatılamadı: %w", err)
	}

	wrote := 0
	var resultErr string
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for sc.Scan() {
		var line cliLine
		if json.Unmarshal(sc.Bytes(), &line) != nil {
			continue
		}
		if line.SessionID != "" {
			c.sessionID = line.SessionID
		}
		switch line.Type {
		case "stream_event":
			if line.Event != nil && line.Event.Delta != nil && line.Event.Delta.Type == "text_delta" {
				fmt.Fprint(out, line.Event.Delta.Text)
				wrote += len(line.Event.Delta.Text)
			}
		case "assistant":
			// fallback for versions that don't emit partial deltas
			if wrote == 0 && line.Message != nil {
				for _, blk := range line.Message.Content {
					if blk.Type == "text" {
						fmt.Fprint(out, blk.Text)
						wrote += len(blk.Text)
					}
				}
			}
		case "result":
			if line.IsError {
				resultErr = line.Result
			}
		}
	}
	waitErr := cmd.Wait()
	if resultErr != "" {
		if strings.Contains(resultErr, "Not logged in") {
			return fmt.Errorf("claude cli oturumu yok — terminalde `claude` çalıştırıp /login yap")
		}
		return fmt.Errorf("claude cli: %s", resultErr)
	}
	if waitErr != nil {
		return fmt.Errorf("claude cli: %w (%s)", waitErr, strings.TrimSpace(stderr.String()))
	}
	if wrote == 0 {
		fmt.Fprint(out, "(boş cevap)")
	}
	return nil
}

func envWithoutAPIKeys() []string {
	var env []string
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "ANTHROPIC_API_KEY=") || strings.HasPrefix(kv, "ANTHROPIC_AUTH_TOKEN=") {
			continue
		}
		env = append(env, kv)
	}
	return env
}

func chatDir() string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".local", "share", "kasa", "chat")
	_ = os.MkdirAll(dir, 0o755)
	return dir
}
