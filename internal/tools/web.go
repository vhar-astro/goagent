package tools

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const ToolNameFetchURL = "fetch_url"

var (
	ErrWebFetchURLRequired = errors.New("web fetch url is required")
	ErrWebFetchTimeout     = errors.New("web fetch timed out")
)

// WebFetchInput is the built-in explicit URL fetch payload.
type WebFetchInput struct {
	URL string `json:"url"`
}

// WebFetchResult captures one explicit URL fetch response.
type WebFetchResult struct {
	URL         string
	FinalURL    string
	StatusCode  int
	Status      string
	ContentType string
	Content     LimitedText
}

// BuiltinWebSpec describes the default direct-URL web fetch tool.
func BuiltinWebSpec() Spec {
	return Spec{
		Name:        ToolNameFetchURL,
		Description: "Fetch one explicit http or https URL and return truncated raw content.",
		Capability:  CapabilityWeb,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url": map[string]any{
					"type":        "string",
					"description": "Absolute http or https URL to fetch.",
				},
			},
			"required":             []string{"url"},
			"additionalProperties": false,
		},
	}
}

// ParseWebFetchInputJSON decodes one provider tool-call argument payload.
func ParseWebFetchInputJSON(raw string) (WebFetchInput, error) {
	var input WebFetchInput
	if err := decodeJSONArguments(raw, &input); err != nil {
		return WebFetchInput{}, fmt.Errorf("decode web fetch arguments: %w", err)
	}

	return input, nil
}

// ExecuteWebFetch performs one explicit URL fetch with URL validation,
// timeout enforcement, and content truncation.
func ExecuteWebFetch(ctx context.Context, runtime webRuntime, input WebFetchInput) (WebFetchResult, error) {
	rawURL := strings.TrimSpace(input.URL)
	if rawURL == "" {
		return WebFetchResult{}, ErrWebFetchURLRequired
	}

	parsedURL, err := runtime.ValidateFetchURL(rawURL)
	if err != nil {
		return WebFetchResult{}, fmt.Errorf("validate fetch url %q: %w", rawURL, err)
	}

	fetchCtx, cancel := runtime.WithWebTimeout(ctx)
	defer cancel()

	request, err := http.NewRequestWithContext(fetchCtx, http.MethodGet, parsedURL.String(), nil)
	if err != nil {
		return WebFetchResult{}, fmt.Errorf("create web fetch request for %q: %w", parsedURL.String(), err)
	}
	request.Header.Set("User-Agent", "goagent/0")

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		if errors.Is(fetchCtx.Err(), context.DeadlineExceeded) {
			return WebFetchResult{}, fmt.Errorf("%w after %s for %q", ErrWebFetchTimeout, runtime.WebTimeout(), parsedURL.String())
		}
		return WebFetchResult{}, fmt.Errorf("fetch url %q: %w", parsedURL.String(), err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return WebFetchResult{}, fmt.Errorf("read response body for %q: %w", parsedURL.String(), err)
	}

	result := WebFetchResult{
		URL:         parsedURL.String(),
		FinalURL:    response.Request.URL.String(),
		StatusCode:  response.StatusCode,
		Status:      response.Status,
		ContentType: response.Header.Get("Content-Type"),
		Content:     limitText(string(body), runtime.WebContentLimit()),
	}

	return result, nil
}

// ToolOutput formats the web fetch response for reinjection into session context.
func (r WebFetchResult) ToolOutput() string {
	var builder strings.Builder

	builder.WriteString("url: ")
	builder.WriteString(r.URL)
	if r.FinalURL != "" && r.FinalURL != r.URL {
		builder.WriteString("\nfinal_url: ")
		builder.WriteString(r.FinalURL)
	}
	builder.WriteString("\nstatus: ")
	builder.WriteString(r.Status)
	if r.ContentType != "" {
		builder.WriteString("\ncontent_type: ")
		builder.WriteString(r.ContentType)
	}
	builder.WriteString("\nbody:\n")
	builder.WriteString(r.Content.Text)

	return builder.String()
}

type webRuntime interface {
	ValidateFetchURL(string) (*url.URL, error)
	WithWebTimeout(context.Context) (context.Context, context.CancelFunc)
	WebTimeout() time.Duration
	WebContentLimit() int
}
