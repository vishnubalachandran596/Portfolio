// Package ai wraps NVIDIA NIM's OpenAI-compatible API (build.nvidia.com)
// for the portfolio's RAG chatbot: free-tier embeddings and chat
// completion, no other external AI dependency.
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const baseURL = "https://integrate.api.nvidia.com/v1"

type Client struct {
	apiKey     string
	chatModel  string
	embedModel string
	httpClient *http.Client
}

// New builds a client. chatModel is openai/gpt-oss-20b -- OpenAI's open-
// weight model, chosen over NVIDIA's own nemotron-3.5-lightning because
// its hybrid-reasoning API shape (a separate reasoning_content field on
// the message, distinct from content) is the same across every provider
// that hosts gpt-oss (NVIDIA NIM, Groq, OpenRouter, Fireworks, ...), not
// a NVIDIA-specific request flag -- the most portable free-tier choice
// if this ever needs to move off NVIDIA. embedModel is nemotron-3-embed
// -1b, the model chatbot_chunks.embedding's column dimension (2048, see
// migration 069) is sized for.
//
// NVIDIA retires NIM-hosted models on a rolling basis with no advance
// code-level warning -- nv-embed-v1 AND its apparent same-generation
// replacement nv-embedqa-e5-v5 both returned HTTP 410 Gone with the
// identical "reached its end of life on 2026-08-25" detail message, which
// is why this is now on nemotron-3-embed-1b (released 2026-07-16, a
// newer generation, confirmed via build.nvidia.com rather than NVIDIA's
// docs.api.nvidia.com reference pages -- those had gone stale for the
// retired models too). A production 500 here surfaces in Vercel's
// runtime logs as "chatbot embed question: nvidia api /embeddings:
// status 410/404: ..." -- if that recurs, the fix is checking
// https://build.nvidia.com/nim?q=embed for the model's current
// replacement (cross-check the model's own page for a release date and
// the absence of a deprecation notice, not just that a reference page
// exists), not assuming the code regressed.
func New(apiKey string) *Client {
	return &Client{
		apiKey:     apiKey,
		chatModel:  "openai/gpt-oss-20b",
		embedModel: "nvidia/nemotron-3-embed-1b",
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// Enabled reports whether an NVIDIA API key is configured. The chatbot
// stays hidden until it is -- same pattern as payments.Razorpay.Enabled().
func (c *Client) Enabled() bool { return c.apiKey != "" }

type embedRequest struct {
	Input          []string `json:"input"`
	Model          string   `json:"model"`
	EncodingFormat string   `json:"encoding_format"`
	InputType      string   `json:"input_type"`
	Truncate       string   `json:"truncate"`
}

type embedResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
	Detail string `json:"detail"`
}

// Embed returns one embedding vector per input text. inputType must be
// "query" (a chatbot question) or "passage" (an indexed content chunk) --
// nv-embed-v1 produces different vectors for each role.
func (c *Client) Embed(ctx context.Context, texts []string, inputType string) ([][]float32, error) {
	body, err := json.Marshal(embedRequest{
		Input:          texts,
		Model:          c.embedModel,
		EncodingFormat: "float",
		InputType:      inputType,
		Truncate:       "END",
	})
	if err != nil {
		return nil, err
	}
	var out embedResponse
	if err := c.post(ctx, "/embeddings", body, &out); err != nil {
		return nil, err
	}
	if out.Detail != "" {
		return nil, fmt.Errorf("nvidia embeddings: %s", out.Detail)
	}
	vecs := make([][]float32, len(out.Data))
	for i, d := range out.Data {
		vecs[i] = d.Embedding
	}
	return vecs, nil
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model           string        `json:"model"`
	Messages        []chatMessage `json:"messages"`
	Temperature     float64       `json:"temperature"`
	MaxTokens       int           `json:"max_tokens"`
	ReasoningEffort string        `json:"reasoning_effort"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
			// ReasoningContent carries gpt-oss's internal chain-of-thought,
			// separate from Content -- see the comment on Chat() below.
			ReasoningContent string `json:"reasoning_content"`
		} `json:"message"`
	} `json:"choices"`
	Detail string `json:"detail"`
}

// Chat sends one grounded question (system prompt + user message,
// non-streaming -- the widget shows one reply at a time) and returns the
// model's reply text.
func (c *Client) Chat(ctx context.Context, systemPrompt, userMessage string) (string, error) {
	body, err := json.Marshal(chatRequest{
		Model: c.chatModel,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userMessage},
		},
		Temperature: 0.2,
		MaxTokens:   900,
		// gpt-oss-20b is a hybrid-reasoning model: it returns its internal
		// chain-of-thought in a separate reasoning_content field rather
		// than mixing it into Content (unlike nemotron, which needed a
		// vendor-specific chat_template_kwargs flag to suppress it
		// entirely). reasoning_effort still bounds how many of MaxTokens
		// go to that trace before the model has to commit to an answer --
		// "low" keeps enough budget free that a RAG-length prompt doesn't
		// exhaust MaxTokens before Content is written.
		ReasoningEffort: "low",
	})
	if err != nil {
		return "", err
	}
	var out chatResponse
	if err := c.post(ctx, "/chat/completions", body, &out); err != nil {
		return "", err
	}
	if out.Detail != "" {
		return "", fmt.Errorf("nvidia chat: %s", out.Detail)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("nvidia chat: empty response")
	}
	return out.Choices[0].Message.Content, nil
}

func (c *Client) post(ctx context.Context, path string, body []byte, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("nvidia api %s: status %d: %s", path, resp.StatusCode, string(respBody))
	}
	return json.Unmarshal(respBody, out)
}
