package grove

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Client struct {
	baseURL   string
	token     string
	groveBin  string
	autoStart bool
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL:   strings.TrimRight(baseURL, "/"),
		groveBin:  "grove",
		autoStart: true,
	}
}

func (c *Client) WithTokenFromDir(dir string) *Client {
	data, err := os.ReadFile(filepath.Join(dir, ".grove", ".token"))
	if err == nil {
		c.token = strings.TrimSpace(string(data))
	}
	return c
}

func (c *Client) EnsureRunning(projectDir string) error {
	if err := c.Health(); err == nil {
		return nil
	}
	if !c.autoStart {
		return fmt.Errorf("grove unreachable at %s", c.baseURL)
	}
	cmd := exec.Command(c.groveBin, "serve", "--port", "7777", projectDir)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start grove: %w", err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(500 * time.Millisecond)
		if err := c.Health(); err == nil {
			c.WithTokenFromDir(projectDir)
			return nil
		}
	}
	return fmt.Errorf("grove did not start within 10s")
}

func (c *Client) Health() error {
	resp, err := http.Get(c.baseURL + "/health")
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("grove health: %d", resp.StatusCode)
	}
	return nil
}

type ICRResponse struct {
	SymbolCount int    `json:"symbol_count"`
	Rating      string `json:"rating"`
}

func (c *Client) ICR(intent string) (*ICRResponse, error) {
	var out ICRResponse
	if err := c.post("/icr", map[string]string{"intent": intent}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) addAuth(req *http.Request) {
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
}

func (c *Client) post(path string, body, out any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, c.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	c.addAuth(req)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("grove POST %s: %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
