package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const (
	// DefaultChatCompletionsPath is appended to provider base URLs that already
	// include the version prefix, such as /v1 or /api/v1.
	DefaultChatCompletionsPath = "/chat/completions"

	defaultErrorBodyLimit = 32 * 1024
)

var (
	errMissingBaseURL    = errors.New("provider base URL is required")
	errMissingAPIKey     = errors.New("provider API key is required")
	errMissingModel      = errors.New("provider request model is required")
	errMissingMessages   = errors.New("provider request must include at least one message")
	errMissingToolName   = errors.New("named tool choice requires a tool name")
	errInvalidToolChoice = errors.New("invalid tool choice mode")
)

// HTTPClientConfig defines the transport and auth settings for one provider.
type HTTPClientConfig struct {
	BaseURL      string
	APIKey       string
	ExtraHeaders map[string]string
	EndpointPath string
	HTTPClient   *http.Client
}

// HTTPClient implements the provider Client contract over raw net/http.
type HTTPClient struct {
	baseURL      *url.URL
	apiKey       string
	extraHeaders map[string]string
	endpointPath string
	httpClient   *http.Client
}

// HTTPStatusError captures non-2xx responses from the provider.
type HTTPStatusError struct {
	StatusCode int
	Status     string
	Body       string
}

func (e *HTTPStatusError) Error() string {
	if strings.TrimSpace(e.Body) == "" {
		return fmt.Sprintf("provider request failed with %s", e.Status)
	}

	return fmt.Sprintf("provider request failed with %s: %s", e.Status, e.Body)
}

// NewClient constructs a raw HTTP provider client.
func NewClient(config HTTPClientConfig) (*HTTPClient, error) {
	return NewHTTPClient(config)
}

// NewHTTPClient constructs a raw HTTP provider client.
func NewHTTPClient(config HTTPClientConfig) (*HTTPClient, error) {
	baseURLText := strings.TrimSpace(config.BaseURL)
	if baseURLText == "" {
		return nil, errMissingBaseURL
	}

	baseURL, err := url.Parse(baseURLText)
	if err != nil {
		return nil, fmt.Errorf("parse provider base URL: %w", err)
	}
	if !baseURL.IsAbs() {
		return nil, fmt.Errorf("provider base URL must be absolute: %s", baseURLText)
	}
	if baseURL.Scheme != "http" && baseURL.Scheme != "https" {
		return nil, fmt.Errorf("provider base URL must use http or https: %s", baseURLText)
	}

	apiKey := strings.TrimSpace(config.APIKey)
	if apiKey == "" {
		return nil, errMissingAPIKey
	}

	client := config.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	endpointPath := strings.TrimSpace(config.EndpointPath)
	if endpointPath == "" {
		endpointPath = DefaultChatCompletionsPath
	}
	if !strings.HasPrefix(endpointPath, "/") {
		endpointPath = "/" + endpointPath
	}

	return &HTTPClient{
		baseURL:      cloneURL(baseURL),
		apiKey:       apiKey,
		extraHeaders: cloneHeaders(config.ExtraHeaders),
		endpointPath: endpointPath,
		httpClient:   client,
	}, nil
}

// Chat sends one normalized request to an OpenAI-compatible chat-completions
// endpoint and returns normalized stream events.
func (c *HTTPClient) Chat(ctx context.Context, request Request) (<-chan StreamEvent, error) {
	if err := validateRequest(request); err != nil {
		return nil, err
	}

	wireRequest, err := buildOpenAIChatRequest(request)
	if err != nil {
		return nil, err
	}

	payload, err := json.Marshal(wireRequest)
	if err != nil {
		return nil, fmt.Errorf("marshal provider request: %w", err)
	}

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpointURL(), bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create provider request: %w", err)
	}

	httpRequest.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "application/json")
	if request.Stream {
		httpRequest.Header.Set("Accept", "text/event-stream")
	}
	for key, value := range c.extraHeaders {
		httpRequest.Header.Set(key, value)
	}

	httpResponse, err := c.httpClient.Do(httpRequest)
	if err != nil {
		return nil, fmt.Errorf("execute provider request: %w", err)
	}

	if httpResponse.StatusCode < 200 || httpResponse.StatusCode >= 300 {
		defer httpResponse.Body.Close()
		body, readErr := io.ReadAll(io.LimitReader(httpResponse.Body, defaultErrorBodyLimit))
		if readErr != nil {
			return nil, fmt.Errorf("read provider error response: %w", readErr)
		}

		return nil, &HTTPStatusError{
			StatusCode: httpResponse.StatusCode,
			Status:     httpResponse.Status,
			Body:       strings.TrimSpace(string(body)),
		}
	}

	events := make(chan StreamEvent, 16)
	if request.Stream {
		go c.streamChatResponse(ctx, httpResponse.Body, events)
		return events, nil
	}

	go c.decodeChatResponse(ctx, httpResponse.Body, events)
	return events, nil
}

func (c *HTTPClient) endpointURL() string {
	endpoint := cloneURL(c.baseURL)
	if endpoint.Path == "" {
		endpoint.Path = c.endpointPath
	} else {
		endpoint.Path = strings.TrimRight(endpoint.Path, "/") + c.endpointPath
	}
	endpoint.RawPath = ""
	endpoint.RawQuery = ""
	endpoint.Fragment = ""

	return endpoint.String()
}

func (c *HTTPClient) decodeChatResponse(ctx context.Context, body io.ReadCloser, events chan<- StreamEvent) {
	defer close(events)
	defer body.Close()

	var response openAIChatResponse
	decoder := json.NewDecoder(body)
	if err := decoder.Decode(&response); err != nil {
		sendError(ctx, events, fmt.Errorf("decode provider response: %w", err))
		return
	}

	if !sendEvent(ctx, events, StreamEvent{
		Type:       EventResponseStart,
		ResponseID: response.ID,
		Model:      response.Model,
	}) {
		return
	}

	usage := convertUsage(response.Usage)
	finishReason := FinishReason("")

	for _, choice := range response.Choices {
		if delta := convertMessageToDelta(choice.Message); delta != nil {
			if delta.Role != "" || delta.Content != "" {
				if !sendEvent(ctx, events, StreamEvent{
					Type:       EventMessageDelta,
					ResponseID: response.ID,
					Model:      response.Model,
					Delta:      &MessageDelta{Role: delta.Role, Content: delta.Content},
				}) {
					return
				}
			}
			if len(delta.Tools) > 0 {
				if !sendEvent(ctx, events, StreamEvent{
					Type:       EventToolCallDelta,
					ResponseID: response.ID,
					Model:      response.Model,
					Delta:      &MessageDelta{Tools: delta.Tools},
				}) {
					return
				}
			}
		}

		if choice.FinishReason != "" {
			finishReason = choice.FinishReason
		}
	}

	if usage != nil {
		if !sendEvent(ctx, events, StreamEvent{
			Type:       EventUsage,
			ResponseID: response.ID,
			Model:      response.Model,
			Usage:      usage,
		}) {
			return
		}
	}

	sendEvent(ctx, events, StreamEvent{
		Type:         EventResponseComplete,
		ResponseID:   response.ID,
		Model:        response.Model,
		Usage:        usage,
		FinishReason: finishReason,
	})
}

func (c *HTTPClient) streamChatResponse(ctx context.Context, body io.ReadCloser, events chan<- StreamEvent) {
	defer close(events)
	defer body.Close()

	decoder := newSSEDecoder(body)
	sawStart := false
	var responseID string
	var model string
	var finishReason FinishReason
	var usage *Usage

	for {
		event, err := decoder.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			sendError(ctx, events, fmt.Errorf("decode provider stream: %w", err))
			return
		}

		data := strings.TrimSpace(event.Data)
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			sendEvent(ctx, events, StreamEvent{
				Type:         EventResponseComplete,
				ResponseID:   responseID,
				Model:        model,
				Usage:        usage,
				FinishReason: finishReason,
			})
			return
		}

		var chunk openAIChatChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			sendError(ctx, events, fmt.Errorf("decode provider stream chunk: %w", err))
			return
		}

		if responseID == "" {
			responseID = chunk.ID
		}
		if model == "" {
			model = chunk.Model
		}
		if !sawStart {
			if !sendEvent(ctx, events, StreamEvent{
				Type:       EventResponseStart,
				ResponseID: responseID,
				Model:      model,
			}) {
				return
			}
			sawStart = true
		}

		for _, choice := range chunk.Choices {
			if choice.Delta.Role != "" || choice.Delta.Content != "" {
				if !sendEvent(ctx, events, StreamEvent{
					Type:       EventMessageDelta,
					ResponseID: responseID,
					Model:      model,
					Delta: &MessageDelta{
						Role:    Role(choice.Delta.Role),
						Content: choice.Delta.Content,
					},
				}) {
					return
				}
			}

			if deltas := convertToolCallDeltas(choice.Delta.ToolCalls); len(deltas) > 0 {
				if !sendEvent(ctx, events, StreamEvent{
					Type:       EventToolCallDelta,
					ResponseID: responseID,
					Model:      model,
					Delta:      &MessageDelta{Tools: deltas},
				}) {
					return
				}
			}

			if choice.FinishReason != "" {
				finishReason = choice.FinishReason
			}
		}

		if chunk.Usage != nil {
			usage = convertUsage(chunk.Usage)
			if usage != nil {
				if !sendEvent(ctx, events, StreamEvent{
					Type:       EventUsage,
					ResponseID: responseID,
					Model:      model,
					Usage:      usage,
				}) {
					return
				}
			}
		}
	}

	if !sawStart {
		sendError(ctx, events, io.ErrUnexpectedEOF)
		return
	}

	sendEvent(ctx, events, StreamEvent{
		Type:         EventResponseComplete,
		ResponseID:   responseID,
		Model:        model,
		Usage:        usage,
		FinishReason: finishReason,
	})
}

func validateRequest(request Request) error {
	if strings.TrimSpace(request.Model) == "" {
		return errMissingModel
	}
	if len(request.Messages) == 0 {
		return errMissingMessages
	}

	for i, message := range request.Messages {
		if err := validateMessage(message); err != nil {
			return fmt.Errorf("validate message %d: %w", i, err)
		}
	}

	for i, tool := range request.Tools {
		if err := validateTool(tool); err != nil {
			return fmt.Errorf("validate tool %d: %w", i, err)
		}
	}

	if request.ToolChoice == nil {
		return nil
	}

	switch request.ToolChoice.Mode {
	case ToolChoiceAuto, ToolChoiceNone, ToolChoiceRequired:
		return nil
	case ToolChoiceNamed:
		if strings.TrimSpace(request.ToolChoice.ToolName) == "" {
			return errMissingToolName
		}
		return nil
	default:
		return fmt.Errorf("%w: %q", errInvalidToolChoice, request.ToolChoice.Mode)
	}
}

func validateMessage(message Message) error {
	switch message.Role {
	case RoleSystem, RoleUser, RoleAssistant, RoleTool:
	default:
		return fmt.Errorf("unsupported message role %q", message.Role)
	}

	if message.Role == RoleTool && strings.TrimSpace(message.ToolCallID) == "" {
		return errors.New("tool messages require tool_call_id")
	}

	return nil
}

func validateTool(tool Tool) error {
	if strings.TrimSpace(tool.Name) == "" {
		return errors.New("tool name is required")
	}
	if tool.Type != "" && tool.Type != ToolTypeFunction {
		return fmt.Errorf("unsupported tool type %q", tool.Type)
	}

	return nil
}

func buildOpenAIChatRequest(request Request) (openAIChatRequest, error) {
	messages := make([]openAIChatMessage, 0, len(request.Messages))
	for _, message := range request.Messages {
		messages = append(messages, openAIChatMessage{
			Role:       string(message.Role),
			Content:    message.Content,
			ToolCallID: message.ToolCallID,
			Name:       message.ToolName,
			ToolCalls:  convertToolCalls(message.ToolCalls),
		})
	}

	tools := make([]openAIToolDefinition, 0, len(request.Tools))
	for _, tool := range request.Tools {
		toolType := tool.Type
		if toolType == "" {
			toolType = ToolTypeFunction
		}
		tools = append(tools, openAIToolDefinition{
			Type: string(toolType),
			Function: openAIFunctionDefinition{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.InputSchema,
			},
		})
	}

	wireRequest := openAIChatRequest{
		Model:    request.Model,
		Messages: messages,
		Tools:    tools,
		Stream:   request.Stream,
	}

	if request.ToolChoice != nil {
		toolChoice, err := convertToolChoice(*request.ToolChoice)
		if err != nil {
			return openAIChatRequest{}, err
		}
		wireRequest.ToolChoice = toolChoice
	}

	if request.StreamOptions != nil {
		wireRequest.StreamOptions = &openAIStreamOptions{
			IncludeUsage: request.StreamOptions.IncludeUsage,
		}
	}

	return wireRequest, nil
}

func convertToolChoice(choice ToolChoice) (any, error) {
	switch choice.Mode {
	case ToolChoiceAuto:
		return "auto", nil
	case ToolChoiceNone:
		return "none", nil
	case ToolChoiceRequired:
		return "required", nil
	case ToolChoiceNamed:
		if strings.TrimSpace(choice.ToolName) == "" {
			return nil, errMissingToolName
		}
		return openAIToolChoice{
			Type: string(ToolTypeFunction),
			Function: openAIToolChoiceFunction{
				Name: choice.ToolName,
			},
		}, nil
	default:
		return nil, fmt.Errorf("%w: %q", errInvalidToolChoice, choice.Mode)
	}
}

func convertToolCalls(toolCalls []ToolCall) []openAIToolCall {
	if len(toolCalls) == 0 {
		return nil
	}

	converted := make([]openAIToolCall, 0, len(toolCalls))
	for _, toolCall := range toolCalls {
		toolType := toolCall.Type
		if toolType == "" {
			toolType = ToolTypeFunction
		}
		converted = append(converted, openAIToolCall{
			ID:   toolCall.ID,
			Type: string(toolType),
			Function: openAIFunctionCall{
				Name:      toolCall.Name,
				Arguments: toolCall.ArgumentsJSON,
			},
		})
	}

	return converted
}

func convertToolCallDeltas(toolCalls []openAIChunkToolCall) []ToolCallDelta {
	if len(toolCalls) == 0 {
		return nil
	}

	converted := make([]ToolCallDelta, 0, len(toolCalls))
	for _, toolCall := range toolCalls {
		converted = append(converted, ToolCallDelta{
			Index:          toolCall.Index,
			ID:             toolCall.ID,
			Type:           ToolType(toolCall.Type),
			Name:           toolCall.Function.Name,
			ArgumentsChunk: toolCall.Function.Arguments,
		})
	}

	return converted
}

func convertMessageToDelta(message openAIResponseMessage) *MessageDelta {
	delta := &MessageDelta{
		Role:    Role(message.Role),
		Content: message.Content,
	}
	for _, toolCall := range message.ToolCalls {
		delta.Tools = append(delta.Tools, ToolCallDelta{
			ID:             toolCall.ID,
			Type:           ToolType(toolCall.Type),
			Name:           toolCall.Function.Name,
			ArgumentsChunk: toolCall.Function.Arguments,
		})
	}

	if delta.Role == "" && delta.Content == "" && len(delta.Tools) == 0 {
		return nil
	}

	return delta
}

func convertUsage(usage *openAIUsage) *Usage {
	if usage == nil {
		return nil
	}

	normalized := &Usage{
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		TotalTokens:      usage.TotalTokens,
	}

	if usage.PromptTokensDetails != nil {
		normalized.PromptTokenDetails = &PromptTokenDetails{
			CachedTokens: usage.PromptTokensDetails.CachedTokens,
			AudioTokens:  usage.PromptTokensDetails.AudioTokens,
		}
	}
	if usage.CompletionTokensDetails != nil {
		normalized.CompletionDetails = &CompletionTokenDetails{
			ReasoningTokens:          usage.CompletionTokensDetails.ReasoningTokens,
			AudioTokens:              usage.CompletionTokensDetails.AudioTokens,
			AcceptedPredictionTokens: usage.CompletionTokensDetails.AcceptedPredictionTokens,
			RejectedPredictionTokens: usage.CompletionTokensDetails.RejectedPredictionTokens,
		}
	}

	return normalized
}

func cloneHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}

	cloned := make(map[string]string, len(headers))
	for key, value := range headers {
		cloned[key] = value
	}

	return cloned
}

func cloneURL(value *url.URL) *url.URL {
	if value == nil {
		return nil
	}

	cloned := *value
	return &cloned
}

func sendEvent(ctx context.Context, events chan<- StreamEvent, event StreamEvent) bool {
	select {
	case <-ctx.Done():
		return false
	case events <- event:
		return true
	}
}

func sendError(ctx context.Context, events chan<- StreamEvent, err error) {
	if err == nil {
		return
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		sendEvent(ctx, events, StreamEvent{Type: EventError, Err: err})
		return
	}

	sendEvent(ctx, events, StreamEvent{Type: EventError, Err: err})
}

type sseDecoder struct {
	reader *bufio.Reader
}

type sseEvent struct {
	Event string
	Data  string
}

func newSSEDecoder(reader io.Reader) *sseDecoder {
	return &sseDecoder{reader: bufio.NewReader(reader)}
}

func (d *sseDecoder) Next() (sseEvent, error) {
	var (
		eventName string
		dataLines []string
	)

	for {
		line, err := d.reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				line = strings.TrimRight(line, "\r\n")
				if line != "" {
					if completed, ok := appendSSELine(line, &eventName, &dataLines); ok {
						return completed, nil
					}
				}
				if eventName != "" || len(dataLines) > 0 {
					return sseEvent{Event: eventName, Data: strings.Join(dataLines, "\n")}, nil
				}
			}
			return sseEvent{}, err
		}

		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			if eventName == "" && len(dataLines) == 0 {
				continue
			}
			return sseEvent{Event: eventName, Data: strings.Join(dataLines, "\n")}, nil
		}

		if completed, ok := appendSSELine(line, &eventName, &dataLines); ok {
			return completed, nil
		}
	}
}

func appendSSELine(line string, eventName *string, dataLines *[]string) (sseEvent, bool) {
	if strings.HasPrefix(line, ":") {
		return sseEvent{}, false
	}

	field, value, hasSeparator := strings.Cut(line, ":")
	if !hasSeparator {
		value = ""
	} else if strings.HasPrefix(value, " ") {
		value = value[1:]
	}

	switch field {
	case "event":
		*eventName = value
	case "data":
		*dataLines = append(*dataLines, value)
	}

	return sseEvent{}, false
}

type openAIChatRequest struct {
	Model         string                 `json:"model"`
	Messages      []openAIChatMessage    `json:"messages"`
	Tools         []openAIToolDefinition `json:"tools,omitempty"`
	ToolChoice    any                    `json:"tool_choice,omitempty"`
	Stream        bool                   `json:"stream"`
	StreamOptions *openAIStreamOptions   `json:"stream_options,omitempty"`
}

type openAIChatMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
	Name       string           `json:"name,omitempty"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
}

type openAIToolDefinition struct {
	Type     string                   `json:"type"`
	Function openAIFunctionDefinition `json:"function"`
}

type openAIFunctionDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type openAIToolChoice struct {
	Type     string                   `json:"type"`
	Function openAIToolChoiceFunction `json:"function"`
}

type openAIToolChoiceFunction struct {
	Name string `json:"name"`
}

type openAIStreamOptions struct {
	IncludeUsage bool `json:"include_usage,omitempty"`
}

type openAIToolCall struct {
	ID       string             `json:"id,omitempty"`
	Type     string             `json:"type,omitempty"`
	Function openAIFunctionCall `json:"function"`
}

type openAIFunctionCall struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

type openAIChatResponse struct {
	ID      string                 `json:"id"`
	Model   string                 `json:"model"`
	Choices []openAIResponseChoice `json:"choices"`
	Usage   *openAIUsage           `json:"usage,omitempty"`
}

type openAIResponseChoice struct {
	Message      openAIResponseMessage `json:"message"`
	FinishReason FinishReason          `json:"finish_reason,omitempty"`
}

type openAIResponseMessage struct {
	Role      string           `json:"role,omitempty"`
	Content   string           `json:"content,omitempty"`
	ToolCalls []openAIToolCall `json:"tool_calls,omitempty"`
}

type openAIChatChunk struct {
	ID      string              `json:"id"`
	Model   string              `json:"model"`
	Choices []openAIChunkChoice `json:"choices"`
	Usage   *openAIUsage        `json:"usage,omitempty"`
}

type openAIChunkChoice struct {
	Delta        openAIChunkDelta `json:"delta"`
	FinishReason FinishReason     `json:"finish_reason,omitempty"`
}

type openAIChunkDelta struct {
	Role      string                `json:"role,omitempty"`
	Content   string                `json:"content,omitempty"`
	ToolCalls []openAIChunkToolCall `json:"tool_calls,omitempty"`
}

type openAIChunkToolCall struct {
	Index    int                `json:"index"`
	ID       string             `json:"id,omitempty"`
	Type     string             `json:"type,omitempty"`
	Function openAIFunctionCall `json:"function"`
}

type openAIUsage struct {
	PromptTokens            int                          `json:"prompt_tokens,omitempty"`
	CompletionTokens        int                          `json:"completion_tokens,omitempty"`
	TotalTokens             int                          `json:"total_tokens,omitempty"`
	PromptTokensDetails     *openAIPromptTokenDetails    `json:"prompt_tokens_details,omitempty"`
	CompletionTokensDetails *openAICompletionTokenDetail `json:"completion_tokens_details,omitempty"`
}

type openAIPromptTokenDetails struct {
	CachedTokens int `json:"cached_tokens,omitempty"`
	AudioTokens  int `json:"audio_tokens,omitempty"`
}

type openAICompletionTokenDetail struct {
	ReasoningTokens          int `json:"reasoning_tokens,omitempty"`
	AudioTokens              int `json:"audio_tokens,omitempty"`
	AcceptedPredictionTokens int `json:"accepted_prediction_tokens,omitempty"`
	RejectedPredictionTokens int `json:"rejected_prediction_tokens,omitempty"`
}
