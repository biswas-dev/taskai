package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Client calls the TaskAI REST API as the caller, with their API key.
type Client struct {
	BaseURL   string
	APIKey    string
	AgentName string
	HTTP      *http.Client
	// PollEvery is how often a PDF export is polled; tests shorten it.
	PollEvery time.Duration
}

// APIError is a non-2xx response, passed on to the agent as it came.
type APIError struct {
	Status int
	Body   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("TaskAI API error %d: %s", e.Status, strings.TrimSpace(e.Body))
}

func (c *Client) do(ctx context.Context, method, path string, body any, auth bool) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.BaseURL, "/")+path, reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if auth {
		req.Header.Set("Authorization", "ApiKey "+c.APIKey)
		if c.AgentName != "" {
			req.Header.Set("X-Agent-Name", c.AgentName)
		}
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("TaskAI API unreachable: %w", err)
	}
	if resp.StatusCode/100 != 2 {
		defer resp.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		return nil, &APIError{Status: resp.StatusCode, Body: string(b)}
	}
	return resp, nil
}

// call sends a JSON request and decodes the JSON reply into out (if non-nil).
func (c *Client) call(ctx context.Context, method, path string, body, out any) error {
	return c.callAs(ctx, method, path, body, out, true)
}

func (c *Client) callAs(ctx context.Context, method, path string, body, out any, auth bool) error {
	resp, err := c.do(ctx, method, path, body, auth)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return err
	}
	if out == nil || len(bytes.TrimSpace(b)) == 0 {
		return nil
	}
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("TaskAI API returned unexpected data: %w", err)
	}
	return nil
}

// get decodes a GET into a generic JSON value.
func (c *Client) get(ctx context.Context, path string) (any, error) {
	var out any
	err := c.call(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

func (c *Client) send(ctx context.Context, method, path string, body any) (any, error) {
	var out any
	err := c.call(ctx, method, path, body, &out)
	return out, err
}

var filenameRE = regexp.MustCompile(`filename="([^"]+)"`)

func filename(resp *http.Response, fallback string) string {
	if m := filenameRE.FindStringSubmatch(resp.Header.Get("Content-Disposition")); m != nil {
		return m[1]
	}
	return fallback
}

// markdown downloads a wiki page as its raw Markdown file.
func (c *Client) markdown(ctx context.Context, pageID string) (content, name string, err error) {
	resp, err := c.do(ctx, http.MethodGet, "/api/wiki/pages/"+url.PathEscape(pageID)+"/markdown", nil, true)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	return string(b), filename(resp, "wiki-page.md"), err
}

// pdf starts a PDF export and polls it until the file is ready.
func (c *Client) pdf(ctx context.Context, pageID string) ([]byte, string, error) {
	base := "/api/wiki/pages/" + url.PathEscape(pageID) + "/pdf"
	var job struct {
		JobID string `json:"job_id"`
	}
	if err := c.call(ctx, http.MethodPost, base, nil, &job); err != nil {
		return nil, "", err
	}
	every := c.PollEvery
	if every == 0 {
		every = 2 * time.Second
	}
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, "", ctx.Err()
		case <-time.After(every):
		}
		resp, err := c.do(ctx, http.MethodGet, base+"/"+url.PathEscape(job.JobID), nil, true)
		if err != nil {
			return nil, "", err
		}
		b, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
		resp.Body.Close()
		if err != nil {
			return nil, "", err
		}
		if strings.Contains(resp.Header.Get("Content-Type"), "application/pdf") {
			return b, filename(resp, "wiki-page.pdf"), nil
		}
		var status struct {
			Status string `json:"status"`
			Error  string `json:"error"`
		}
		_ = json.Unmarshal(b, &status)
		if status.Status == "failed" {
			if status.Error == "" {
				status.Error = "PDF generation failed"
			}
			return nil, "", fmt.Errorf("%s", status.Error)
		}
	}
	return nil, "", fmt.Errorf("PDF generation timed out")
}

func esc(s string) string { return url.PathEscape(s) }

// remarshal converts a decoded JSON value into a typed one.
func remarshal(in, out any) error {
	b, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}
