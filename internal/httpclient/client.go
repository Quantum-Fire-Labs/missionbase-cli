package httpclient

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Quantum-Fire-Labs/missionbase-cli/internal/config"
)

type ActorMode string

const (
	ActorUser  ActorMode = "user"
	ActorAgent ActorMode = "agent"
)

type Client struct {
	cfg       config.Config
	actorMode ActorMode
	client    *http.Client
}

func New(cfg config.Config) Client {
	return NewAgent(cfg)
}

func NewUser(cfg config.Config) Client {
	cfg.AgentSlug = ""
	return newWithActor(cfg, ActorUser)
}

func NewAgent(cfg config.Config) Client {
	return newWithActor(cfg, ActorAgent)
}

func newWithActor(cfg config.Config, actorMode ActorMode) Client {
	return Client{
		cfg:       cfg,
		actorMode: actorMode,
		client:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (c Client) Get(path string) ([]byte, error) {
	return c.Do(http.MethodGet, path, nil)
}

func (c Client) Post(path string, body []byte) ([]byte, error) {
	return c.Do(http.MethodPost, path, body)
}

func (c Client) PostWithContentType(path string, body []byte, contentType string) ([]byte, error) {
	return c.do(http.MethodPost, path, body, contentType)
}

func (c Client) Patch(path string, body []byte) ([]byte, error) {
	return c.Do(http.MethodPatch, path, body)
}

func (c Client) PatchWithContentType(path string, body []byte, contentType string) ([]byte, error) {
	return c.do(http.MethodPatch, path, body, contentType)
}

func (c Client) Delete(path string) ([]byte, error) {
	return c.Do(http.MethodDelete, path, nil)
}

func (c Client) Do(method, path string, body []byte) ([]byte, error) {
	contentType := ""
	if body != nil {
		contentType = "application/json"
	}
	return c.do(method, path, body, contentType)
}

func (c Client) do(method, path string, body []byte, contentType string) ([]byte, error) {
	url := strings.TrimRight(c.cfg.BaseURL, "/") + "/" + strings.TrimLeft(path, "/")
	req, err := http.NewRequest(method, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if c.cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	}
	if c.actorMode == ActorAgent && c.cfg.AgentSlug != "" {
		req.Header.Set("X-Missionbase-Agent-Slug", c.cfg.AgentSlug)
	}
	req.Header.Set("Accept", "application/json")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("User-Agent", "missionbase-cli")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s %s failed: HTTP %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return respBody, nil
}
