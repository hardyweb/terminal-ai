package main

import (
	"context"
	"fmt"
	"strings"
	"time"
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

	// Skip if conversation is too short
	if len(conversation) < 50 {
		// fmt.Fprintf(os.Stderr, "[DEBUG] Conversation too short (%d chars), skipping extraction\n", len(conversation))
		return []string{}, nil
	}

	// fmt.Fprintf(os.Stderr, "[DEBUG] Calling AI for extraction...\n")

	prompt := fmt.Sprintf(`Analyze this conversation and extract important facts, preferences, and information that should be remembered for future interactions.

EXTRACT items like:
- Personal information (name, location, work, role)
- User preferences (likes, dislikes, favorite tools, coding style)
- Project details (tech stack, architecture decisions, requirements)
- Important facts mentioned (deadlines, priorities, constraints)
- Configuration details (settings, environment variables, API endpoints)

DO NOT extract:
- Greetings or casual conversation
- Questions without answers
- Temporary or context-specific info
- Information user said to forget

FORMAT: List each item on a new line starting with "- "
If nothing important to remember, respond with "No memories to extract"

CONVERSATION:
%s

MEMORIES:`, conversation)

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

	// fmt.Fprintf(os.Stderr, "[DEBUG] Using provider: %s\n", provider.Name)

	req := Request{
		Model: provider.Model,
		Messages: []Message{
			{Role: "user", Content: prompt},
		},
		Stream: false,
	}

	// fmt.Fprintf(os.Stderr, "[DEBUG] Sending request to %s...\n", provider.Endpoint)
	response, err := makeRequest(provider.Endpoint, provider.APIKey, req, provider.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to extract memories: %w", err)
	}

	// fmt.Fprintf(os.Stderr, "[DEBUG] Response received, checking choices...\n")
	if len(response.Choices) == 0 {
		return nil, fmt.Errorf("no response from AI")
	}

	content := response.Choices[0].Message.Content
	// fmt.Fprintf(os.Stderr, "[DEBUG] AI response content length: %d\n", len(content))
	// fmt.Fprintf(os.Stderr, "[DEBUG] AI response: %s\n", content)

	lines := strings.Split(content, "\n")
	var memories []string

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Skip empty lines and lines that are just dashes/asterisks
		if len(line) <= 1 {
			continue
		}

		// Remove bullet markers
		line = strings.TrimPrefix(line, "- ")
		line = strings.TrimPrefix(line, "* ")
		line = strings.TrimPrefix(line, "-")
		line = strings.TrimPrefix(line, "*")
		line = strings.TrimSpace(line)

		// Skip if still too short after trimming
		if len(line) < 10 {
			continue
		}

		// Skip common non-memory responses
		lowerLine := strings.ToLower(line)
		if lowerLine == "none" || lowerLine == "no memories" || lowerLine == "n/a" || lowerLine == "-" {
			continue
		}

		if len(line) < 500 {
			memories = append(memories, line)
		}
	}

	// fmt.Fprintf(os.Stderr, "[DEBUG] Parsed %d memories\n", len(memories))

	// Limit to max 10 memories
	if len(memories) > 10 {
		// fmt.Fprintf(os.Stderr, "[DEBUG] Limiting to top 10 memories\n")
		memories = memories[:10]
	}

	return memories, nil
}

func (e *AutoMemoryExtractor) SaveExtractedMemories(ctx context.Context, memories []string, sessionID string) (int, error) {
	if e.mgr == nil {
		return 0, fmt.Errorf("memory manager not initialized")
	}

	// Limit to max 10 most important memories to prevent overload
	if len(memories) > 10 {
		// fmt.Fprintf(os.Stderr, "[DEBUG] Limiting %d memories to top 10\n", len(memories))
		memories = memories[:10]
	}

	// fmt.Fprintf(os.Stderr, "[DEBUG] Starting to save %d memories...\n", len(memories))

	saved := 0
	startTime := time.Now()

	for i, memory := range memories {
		// Quick duplicate check - skip exact matches only
		existing, err := e.mgr.base.SearchMemories(ctx, memory, 1)
		if err == nil && len(existing) > 0 {
			existingContent := existing[0].Memory.Content
			if memory == existingContent || strings.Contains(existingContent, memory) {
				// fmt.Fprintf(os.Stderr, "[DEBUG] Skipping duplicate %d/%d\n", i+1, len(memories))
				continue
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

		// Progress update every 3 memories
		if (i+1)%3 == 0 {
			// fmt.Fprintf(os.Stderr, "[DEBUG] Saved %d/%d memories...\n", i+1, len(memories))
		}
	}

	_ = time.Since(startTime) // Avoid unused variable

	return saved, nil
}

func (e *AutoMemoryExtractor) ProcessConversation(ctx context.Context, conversation string, sessionID string) (int, error) {
	// fmt.Fprintf(os.Stderr, "[DEBUG] Starting extraction...\n")
	memories, err := e.ExtractFromConversation(ctx, conversation, sessionID)
	if err != nil {
		// fmt.Fprintf(os.Stderr, "[DEBUG] ExtractFromConversation failed: %v\n", err)
		return 0, err
	}
	// fmt.Fprintf(os.Stderr, "[DEBUG] Got %d memories from AI\n", len(memories))

	count, saveErr := e.SaveExtractedMemories(ctx, memories, sessionID)
	if saveErr != nil {
		// fmt.Fprintf(os.Stderr, "[DEBUG] SaveExtractedMemories failed: %v\n", saveErr)
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
		// fmt.Fprintf(os.Stderr, "[DEBUG] No extractor available\n")
		return 0
	}

	count, err := extractor.ProcessConversation(ctx, conversation, sessionID)
	if err != nil {
		// fmt.Fprintf(os.Stderr, "[DEBUG] Extraction failed: %v\n", err)
		return 0
	}

	// fmt.Fprintf(os.Stderr, "[DEBUG] Processed %d memories\n", count)
	return count
}
