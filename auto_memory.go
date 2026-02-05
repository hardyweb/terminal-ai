package main

import (
	"context"
	"fmt"
	"os"
	"strings"
)

type AutoMemoryExtractor struct {
	mgr      *EncryptedMemoryManager
	minScore float32
	keywords []string
}

func NewAutoMemoryExtractor(mgr *EncryptedMemoryManager) *AutoMemoryExtractor {
	return &AutoMemoryExtractor{
		mgr:      mgr,
		minScore: 0.7,
		keywords: []string{
			"remember", "don't forget", "important", "note that",
			"my name is", "i am", "i'm", "call me",
			"i work as", "i work at", "i live in", "my favorite",
			"i prefer", "i like", "i dislike", "i hate",
			"always", "never", "must", "should",
			"password", "api key", "secret", "credential",
			"preference", "setting", "configuration", "config",
		},
	}
}

func (e *AutoMemoryExtractor) ExtractFromConversation(ctx context.Context, conversation string, sessionID string) ([]string, error) {
	if e.mgr == nil {
		return nil, fmt.Errorf("memory manager not initialized")
	}

	fmt.Fprintf(os.Stderr, "[DEBUG] Calling AI for extraction...\n")

	prompt := fmt.Sprintf(`Extract important facts, preferences, and information from this conversation that should be remembered long-term. 

Rules:
1. Only extract information that is explicitly stated as important
2. Extract personal details (name, location, work, preferences)
3. Extract project-specific information
4. Extract user preferences and likes/dislikes
5. Extract technical details and configurations
6. Do NOT extract casual conversation or greetings
7. Do NOT extract information that the user explicitly said to forget
8. Keep each item concise (under 50 words)

Format each item on a separate line, starting with a dash.

Conversation:
%s

Extracted memories:`, conversation)

	provider := providers["openrouter"]
	if provider.APIKey == "" {
		provider = providers["gemini"]
	}

	if provider.APIKey == "" {
		provider = providers["groq"]
	}

	if provider.APIKey == "" {
		return nil, fmt.Errorf("no API key configured")
	}

	fmt.Fprintf(os.Stderr, "[DEBUG] Using provider: %s\n", provider.Name)

	req := Request{
		Model: provider.Model,
		Messages: []Message{
			{Role: "user", Content: prompt},
		},
		Stream: false,
	}

	fmt.Fprintf(os.Stderr, "[DEBUG] Sending request to %s...\n", provider.Endpoint)
	response, err := makeRequest(provider.Endpoint, provider.APIKey, req, provider.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to extract memories: %w", err)
	}

	fmt.Fprintf(os.Stderr, "[DEBUG] Response received, checking choices...\n")
	if len(response.Choices) == 0 {
		return nil, fmt.Errorf("no response from AI")
	}

	content := response.Choices[0].Message.Content
	fmt.Fprintf(os.Stderr, "[DEBUG] AI response content length: %d\n", len(content))
	fmt.Fprintf(os.Stderr, "[DEBUG] AI response: %s\n", content)

	lines := strings.Split(content, "\n")
	var memories []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "- ")
		line = strings.TrimPrefix(line, "* ")

		if len(line) > 10 && len(line) < 500 {
			memories = append(memories, line)
		}
	}

	fmt.Fprintf(os.Stderr, "[DEBUG] Parsed %d memories\n", len(memories))
	return memories, nil
}

func (e *AutoMemoryExtractor) SaveExtractedMemories(ctx context.Context, memories []string, sessionID string) (int, error) {
	if e.mgr == nil {
		return 0, fmt.Errorf("memory manager not initialized")
	}

	saved := 0
	for _, memory := range memories {
		existing, err := e.mgr.base.SearchMemories(ctx, memory, 1)
		if err == nil && len(existing) > 0 {
			for _, result := range existing {
				if strings.Contains(result.Memory.Content, memory) || strings.Contains(memory, result.Memory.Content) {
					continue
				}
			}
		}

		metadata := MemoryMetadata{
			Source:    "auto-extract",
			SessionID: sessionID,
			Tags:      []string{"auto-extracted"},
		}

		_, addErr := e.mgr.AddEncryptedMemory(ctx, memory, metadata)
		if addErr == nil {
			saved++
		}
	}

	return saved, nil
}

func (e *AutoMemoryExtractor) ProcessConversation(ctx context.Context, conversation string, sessionID string) (int, error) {
	fmt.Fprintf(os.Stderr, "[DEBUG] Starting extraction...\n")
	memories, err := e.ExtractFromConversation(ctx, conversation, sessionID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[DEBUG] ExtractFromConversation failed: %v\n", err)
		return 0, err
	}
	fmt.Fprintf(os.Stderr, "[DEBUG] Got %d memories from AI\n", len(memories))

	count, saveErr := e.SaveExtractedMemories(ctx, memories, sessionID)
	if saveErr != nil {
		fmt.Fprintf(os.Stderr, "[DEBUG] SaveExtractedMemories failed: %v\n", saveErr)
		return count, saveErr
	}

	return count, nil
}

func (e *AutoMemoryExtractor) SetMinScore(score float32) {
	e.minScore = score
}

func (e *AutoMemoryExtractor) AddKeyword(keyword string) {
	e.keywords = append(e.keywords, keyword)
}

func (e *AutoMemoryExtractor) HasImportantContent(text string) bool {
	lowerText := strings.ToLower(text)
	for _, keyword := range e.keywords {
		if strings.Contains(lowerText, keyword) {
			return true
		}
	}
	return false
}

var autoExtractor *AutoMemoryExtractor

func GetAutoMemoryExtractor() *AutoMemoryExtractor {
	return autoExtractor
}

func InitAutoMemoryExtractor() {
	mgr := GetEncryptedMemoryManager()
	if mgr != nil {
		autoExtractor = NewAutoMemoryExtractor(mgr)
	}
}

func ExtractAndSaveMemories(conversation string, sessionID string) int {
	ctx := context.Background()
	extractor := GetAutoMemoryExtractor()
	if extractor == nil {
		fmt.Fprintf(os.Stderr, "[DEBUG] No extractor available\n")
		return 0
	}

	count, err := extractor.ProcessConversation(ctx, conversation, sessionID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[DEBUG] Extraction failed: %v\n", err)
		return 0
	}

	fmt.Fprintf(os.Stderr, "[DEBUG] Processed %d memories\n", count)
	return count
}
