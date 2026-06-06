package provider

import "context"

// Role identifies the sender for one normalized chat message.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// ToolType identifies the OpenAI-compatible tool family in use.
type ToolType string

const (
	ToolTypeFunction ToolType = "function"
)

// ToolChoiceMode models the supported OpenAI-compatible tool choice modes.
type ToolChoiceMode string

const (
	ToolChoiceAuto     ToolChoiceMode = "auto"
	ToolChoiceNone     ToolChoiceMode = "none"
	ToolChoiceRequired ToolChoiceMode = "required"
	ToolChoiceNamed    ToolChoiceMode = "named"
)

// FinishReason describes why a chat completion stopped producing output.
type FinishReason string

const (
	FinishReasonStop          FinishReason = "stop"
	FinishReasonToolCalls     FinishReason = "tool_calls"
	FinishReasonLength        FinishReason = "length"
	FinishReasonContentFilter FinishReason = "content_filter"
)

// Message is the normalized chat message shape shared across providers.
type Message struct {
	Role       Role       `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolName   string     `json:"tool_name,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
}

// Tool describes one callable tool exposed to the active provider. Type should
// default to ToolTypeFunction for current OpenAI-compatible providers.
type Tool struct {
	Type        ToolType       `json:"type,omitempty"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"input_schema,omitempty"`
}

// ToolChoice optionally forces a specific tool or mode for the next request.
type ToolChoice struct {
	Mode     ToolChoiceMode `json:"mode,omitempty"`
	ToolName string         `json:"tool_name,omitempty"`
}

// StreamOptions controls provider-specific streaming features.
type StreamOptions struct {
	IncludeUsage bool `json:"include_usage,omitempty"`
}

// ToolCall captures one fully assembled provider-requested tool invocation.
// ArgumentsJSON stays as raw JSON text because providers stream function
// arguments incrementally as string fragments.
type ToolCall struct {
	ID            string   `json:"id,omitempty"`
	Type          ToolType `json:"type,omitempty"`
	Name          string   `json:"name,omitempty"`
	ArgumentsJSON string   `json:"arguments_json,omitempty"`
}

// Request is the provider-agnostic chat request consumed by adapters.
type Request struct {
	Model         string         `json:"model"`
	Messages      []Message      `json:"messages"`
	Tools         []Tool         `json:"tools,omitempty"`
	ToolChoice    *ToolChoice    `json:"tool_choice,omitempty"`
	Stream        bool           `json:"stream"`
	StreamOptions *StreamOptions `json:"stream_options,omitempty"`
}

// Response is the fully assembled provider result after streaming completes.
type Response struct {
	ID           string       `json:"id,omitempty"`
	Model        string       `json:"model,omitempty"`
	Message      Message      `json:"message"`
	FinishReason FinishReason `json:"finish_reason,omitempty"`
	Usage        Usage        `json:"usage"`
}

// Usage records token accounting returned by the provider.
type Usage struct {
	PromptTokens       int                     `json:"prompt_tokens,omitempty"`
	CompletionTokens   int                     `json:"completion_tokens,omitempty"`
	TotalTokens        int                     `json:"total_tokens,omitempty"`
	PromptTokenDetails *PromptTokenDetails     `json:"prompt_token_details,omitempty"`
	CompletionDetails  *CompletionTokenDetails `json:"completion_details,omitempty"`
}

// PromptTokenDetails keeps optional provider token-accounting extensions.
type PromptTokenDetails struct {
	CachedTokens int `json:"cached_tokens,omitempty"`
	AudioTokens  int `json:"audio_tokens,omitempty"`
}

// CompletionTokenDetails keeps optional provider completion token extensions.
type CompletionTokenDetails struct {
	ReasoningTokens          int `json:"reasoning_tokens,omitempty"`
	AudioTokens              int `json:"audio_tokens,omitempty"`
	AcceptedPredictionTokens int `json:"accepted_prediction_tokens,omitempty"`
	RejectedPredictionTokens int `json:"rejected_prediction_tokens,omitempty"`
}

// StreamEventType describes the kind of incremental provider event observed.
type StreamEventType string

const (
	EventResponseStart    StreamEventType = "response_start"
	EventMessageDelta     StreamEventType = "message_delta"
	EventToolCallDelta    StreamEventType = "tool_call_delta"
	EventUsage            StreamEventType = "usage"
	EventResponseComplete StreamEventType = "response_complete"
	EventError            StreamEventType = "error"
)

// MessageDelta represents one incremental assistant update in a stream chunk.
type MessageDelta struct {
	Role    Role            `json:"role,omitempty"`
	Content string          `json:"content,omitempty"`
	Tools   []ToolCallDelta `json:"tool_calls,omitempty"`
}

// ToolCallDelta captures partial tool-call updates by index as they stream.
// ArgumentsChunk is appended to build the final ToolCall.ArgumentsJSON value.
type ToolCallDelta struct {
	Index          int      `json:"index"`
	ID             string   `json:"id,omitempty"`
	Type           ToolType `json:"type,omitempty"`
	Name           string   `json:"name,omitempty"`
	ArgumentsChunk string   `json:"arguments_chunk,omitempty"`
}

// StreamEvent represents one chunk emitted while provider output is streaming.
type StreamEvent struct {
	Type         StreamEventType
	ResponseID   string
	Model        string
	Delta        *MessageDelta
	Usage        *Usage
	FinishReason FinishReason
	Err          error
}

// Client is the minimal provider adapter contract for chat streaming.
type Client interface {
	Chat(context.Context, Request) (<-chan StreamEvent, error)
}
