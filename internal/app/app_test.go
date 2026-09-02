package app

import (
	"context"
	"io"
	"testing"

	"github.com/vhar-astro/goagent/internal/provider"
	"github.com/vhar-astro/goagent/internal/session"
)

func TestSessionPromptSubmitterForwardsTurnUsageToAssistantStream(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	runtime, err := NewRuntime(workspace)
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}

	client := &stubProviderClient{responses: []provider.Response{
		stubTextResponse("done", provider.Usage{PromptTokens: 120, CompletionTokens: 45, TotalTokens: 165}),
	}}

	submitter := &sessionPromptSubmitter{
		session: session.New("session-1", workspace, "chutes", "model"),
		runtime: runtime,
		stream:  true,
		clientFactory: func() (provider.Client, error) {
			return client, nil
		},
	}

	stream, err := submitter.SubmitPrompt(context.Background(), "hello")
	if err != nil {
		t.Fatalf("SubmitPrompt() error = %v", err)
	}

	var (
		texts []string
		usage *provider.Usage
	)
	for {
		chunk, err := stream.Recv(context.Background())
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Recv() error = %v", err)
		}
		if chunk.Text != "" {
			texts = append(texts, chunk.Text)
		}
		if chunk.Usage != nil {
			usage = &provider.Usage{
				PromptTokens:     chunk.Usage.InputTokens,
				CompletionTokens: chunk.Usage.OutputTokens,
				TotalTokens:      chunk.Usage.TotalTokens,
			}
		}
	}

	if len(texts) != 1 || texts[0] != "done" {
		t.Fatalf("streamed texts = %#v, want [done]", texts)
	}
	if usage == nil {
		t.Fatal("usage chunk = nil, want forwarded token usage")
	}
	if usage.PromptTokens != 120 || usage.CompletionTokens != 45 || usage.TotalTokens != 165 {
		t.Fatalf("usage = %+v, want prompt=120 completion=45 total=165", *usage)
	}
}
