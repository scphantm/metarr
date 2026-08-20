package provider

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"Metarr/internal/shared/appconfig"
)

// buildVersion identifies this process to the MCP server it connects to.
// Not tied to the project's actual release versioning — MCP servers only
// use it for logging/diagnostics on their side.
const buildVersion = "0.1.0"

// mcpProvider adapts a chat-capable MCP tool to Provider. MCP's CallTool is
// single request/response, not a native token stream, so Stream synthesizes
// a handful of Deltas by chunking the tool's returned text — indistinguishable
// downstream from the three natively-streaming providers.
type mcpProvider struct {
	cfg appconfig.ChatbotMCPConfig

	mu      sync.Mutex
	client  *mcp.Client
	session *mcp.ClientSession
}

func newMCPProvider(cfg appconfig.ChatbotMCPConfig) Provider {
	return &mcpProvider{
		cfg:    cfg,
		client: mcp.NewClient(&mcp.Implementation{Name: "metarr", Version: buildVersion}, nil),
	}
}

// connect returns the current session, establishing one if none is live —
// one long-lived connection per server process, reconnected lazily on the
// next completion request if it died, rather than reconnecting (or, for
// stdio, respawning a subprocess) per chat message.
func (p *mcpProvider) connect(ctx context.Context) (*mcp.ClientSession, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.session != nil {
		return p.session, nil
	}

	var transport mcp.Transport
	switch p.cfg.Transport {
	case "stdio":
		transport = &mcp.CommandTransport{Command: exec.Command(p.cfg.Command, p.cfg.Args...)}
	case "http":
		transport = &mcp.StreamableClientTransport{Endpoint: p.cfg.URL}
	default:
		return nil, fmt.Errorf("unknown mcp transport %q", p.cfg.Transport)
	}

	session, err := p.client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, err
	}
	p.session = session
	return session, nil
}

func (p *mcpProvider) dropSession() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.session = nil
}

func (p *mcpProvider) Stream(ctx context.Context, req CompletionRequest, emit func(Delta)) error {
	session, err := p.connect(ctx)
	if err != nil {
		emit(Delta{Done: true, Err: err})
		return err
	}

	messages := make([]map[string]string, 0, len(req.Messages))
	for _, m := range req.Messages {
		messages = append(messages, map[string]string{"role": m.Role, "content": m.Content})
	}

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: p.cfg.ToolName,
		Arguments: map[string]any{
			"messages": messages,
			"system":   req.System,
		},
	})
	if err != nil {
		// The session may have died between calls; drop it so the next
		// Stream call reconnects instead of repeating the same failure.
		p.dropSession()
		emit(Delta{Done: true, Err: err})
		return err
	}

	var text strings.Builder
	for _, content := range result.Content {
		if tc, ok := content.(*mcp.TextContent); ok {
			text.WriteString(tc.Text)
		}
	}

	// Chunk the single response into a few Deltas on word boundaries so the
	// UI still renders it incrementally, matching the other providers' feel.
	for _, chunk := range chunkWords(text.String(), 8) {
		emit(Delta{Text: chunk})
	}

	if result.IsError {
		err := fmt.Errorf("mcp tool %q returned an error result", p.cfg.ToolName)
		emit(Delta{Done: true, Err: err})
		return err
	}

	emit(Delta{Done: true})
	return nil
}

// chunkWords splits text into groups of n words, preserving the trailing
// whitespace between them so re-joining the chunks reproduces the original.
func chunkWords(text string, n int) []string {
	if text == "" {
		return nil
	}
	fields := strings.SplitAfter(text, " ")
	chunks := make([]string, 0, (len(fields)/n)+1)
	var b strings.Builder
	for i, f := range fields {
		b.WriteString(f)
		if (i+1)%n == 0 {
			chunks = append(chunks, b.String())
			b.Reset()
		}
	}
	if b.Len() > 0 {
		chunks = append(chunks, b.String())
	}
	return chunks
}
