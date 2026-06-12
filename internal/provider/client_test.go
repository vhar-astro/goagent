package provider

import (
	"context"
	"io"
	"strings"
	"testing"
)

func TestStreamChatResponseEmitsFinalUsageChunkBeforeCompletion(t *testing.T) {
	t.Parallel()

	streamBody := strings.NewReader(strings.Join([]string{
		"data: {\"id\":\"resp-1\",\"model\":\"gpt-test\",\"choices\":[{\"delta\":{\"role\":\"assistant\",\"content\":\"Hello\"}}]}",
		"",
		"data: {\"id\":\"resp-1\",\"model\":\"gpt-test\",\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}",
		"",
		"data: {\"id\":\"resp-1\",\"model\":\"gpt-test\",\"choices\":[],\"usage\":{\"prompt_tokens\":120,\"completion_tokens\":45,\"total_tokens\":165}}",
		"",
		"data: [DONE]",
		"",
	}, "\n"))

	client := &HTTPClient{}
	events := make(chan StreamEvent, 8)
	go client.streamChatResponse(context.Background(), io.NopCloser(streamBody), events)

	var got []StreamEvent
	for event := range events {
		got = append(got, event)
	}

	if len(got) != 4 {
		t.Fatalf("event count = %d, want 4", len(got))
	}
	if got[0].Type != EventResponseStart {
		t.Fatalf("event[0].Type = %q, want %q", got[0].Type, EventResponseStart)
	}
	if got[1].Type != EventMessageDelta || got[1].Delta == nil || got[1].Delta.Content != "Hello" {
		t.Fatalf("event[1] = %+v, want assistant content delta", got[1])
	}
	if got[2].Type != EventUsage || got[2].Usage == nil {
		t.Fatalf("event[2] = %+v, want usage event", got[2])
	}
	if got[2].Usage.PromptTokens != 120 || got[2].Usage.CompletionTokens != 45 || got[2].Usage.TotalTokens != 165 {
		t.Fatalf("event[2].Usage = %+v, want prompt=120 completion=45 total=165", got[2].Usage)
	}
	if got[3].Type != EventResponseComplete || got[3].Usage == nil {
		t.Fatalf("event[3] = %+v, want completion event with usage", got[3])
	}
	if got[3].Usage.PromptTokens != 120 || got[3].Usage.CompletionTokens != 45 || got[3].Usage.TotalTokens != 165 {
		t.Fatalf("event[3].Usage = %+v, want prompt=120 completion=45 total=165", got[3].Usage)
	}
}
