package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/philippgille/chromem-go"
)

type Memory struct {
	ID         string         `json:"id"`
	Content    string         `json:"content"`
	Embedding  []float32      `json:"-"`
	Metadata   MemoryMetadata `json:"metadata"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	Importance float32        `json:"importance"`
}

type MemoryMetadata struct {
	Source      string   `json:"source"`
	SessionID   string   `json:"session_id"`
	User        string   `json:"user"`
	Tags        []string `json:"tags"`
	IsEncrypted bool     `json:"is_encrypted"`
}

type MemorySearchResult struct {
	Memory     Memory
	Similarity float32
}

type MemoryManager struct {
	db          *chromem.DB
	collection  *chromem.Collection
	embeddings  *EmbeddingService
	dataDir     string
	initialized bool
}

type EmbeddingService struct {
	apiURL  string
	model   string
	timeout time.Duration
}

const (
	OpenRouterEmbeddingsURL = "https://openrouter.ai/api/v1/embeddings"
	MemoryDBFileName        = "memory.db"
	MemoryCollectionName    = "memories"
	DefaultTopK             = 5
	DefaultImportance       = 0.5
)

var memoryMgr *MemoryManager

func NewEmbeddingService() *EmbeddingService {
	return &EmbeddingService{
		apiURL:  OpenRouterEmbeddingsURL,
		model:   "text-embedding-3-small",
		timeout: 60 * time.Second,
	}
}

func (e *EmbeddingService) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
	payload := map[string]interface{}{
		"model": e.model,
		"input": []string{text},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", e.apiURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("OPENROUTER_API_KEY not set")
	}
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil, fmt.Errorf("OPENROUTER_API_KEY is empty")
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("HTTP-Referer", "https://github.com/user/terminal-ai")
	req.Header.Set("X-Title", "Terminal AI")

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call embedding API: %w", err)
	}
	defer resp.Body.Close()

	bodyResp, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("embedding API returned status %d: %s", resp.StatusCode, string(bodyResp))
	}

	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(bodyResp, &result); err != nil {
		return nil, fmt.Errorf("failed to decode embeddings: %w", err)
	}

	if len(result.Data) == 0 || len(result.Data[0].Embedding) == 0 {
		return nil, fmt.Errorf("no embeddings returned")
	}

	return result.Data[0].Embedding, nil
}

func InitMemoryManager(dataDir string) error {
	memoryDataDir := filepath.Join(dataDir, "memory")
	if err := os.MkdirAll(memoryDataDir, 0700); err != nil {
		return fmt.Errorf("failed to create memory data directory: %w", err)
	}

	dbPath := filepath.Join(memoryDataDir, MemoryDBFileName)

	db, err := chromem.NewPersistentDB(dbPath, false)
	if err != nil {
		return fmt.Errorf("failed to create vector database: %w", err)
	}

	collection, err := db.GetOrCreateCollection(MemoryCollectionName, nil, nil)
	if err != nil {
		db.Reset()
		return fmt.Errorf("failed to get/create collection: %w", err)
	}

	memoryMgr = &MemoryManager{
		db:          db,
		collection:  collection,
		embeddings:  NewEmbeddingService(),
		dataDir:     memoryDataDir,
		initialized: true,
	}

	return nil
}

func GetMemoryManager() *MemoryManager {
	return memoryMgr
}

func (m *MemoryManager) Close() error {
	if m.db != nil {
		return m.db.Reset()
	}
	return nil
}

func (m *MemoryManager) AddMemory(ctx context.Context, content string, metadata MemoryMetadata) (*Memory, error) {
	if !m.initialized {
		return nil, fmt.Errorf("memory manager not initialized")
	}

	embedding, err := m.embeddings.GenerateEmbedding(ctx, content)
	if err != nil {
		return nil, fmt.Errorf("failed to generate embedding: %w", err)
	}

	memory := &Memory{
		ID:         generateUUID(),
		Content:    content,
		Embedding:  embedding,
		Metadata:   metadata,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		Importance: DefaultImportance,
	}

	docMetadata := map[string]string{
		"created_at":   memory.CreatedAt.Format(time.RFC3339),
		"updated_at":   memory.UpdatedAt.Format(time.RFC3339),
		"importance":   fmt.Sprintf("%f", memory.Importance),
		"source":       metadata.Source,
		"session_id":   metadata.SessionID,
		"user":         metadata.User,
		"tags":         strings.Join(metadata.Tags, ","),
		"is_encrypted": fmt.Sprintf("%v", metadata.IsEncrypted),
	}

	doc, err := chromem.NewDocument(ctx, memory.ID, docMetadata, embedding, content, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create document: %w", err)
	}

	if err := m.collection.AddDocument(ctx, doc); err != nil {
		return nil, fmt.Errorf("failed to add document to collection: %w", err)
	}

	return memory, nil
}

func (m *MemoryManager) SearchMemories(ctx context.Context, query string, topK int) ([]MemorySearchResult, error) {
	if !m.initialized {
		return nil, fmt.Errorf("memory manager not initialized")
	}

	count := m.collection.Count()
	if topK <= 0 || topK > count {
		topK = count
	}
	if topK == 0 {
		return []MemorySearchResult{}, nil
	}

	embedding, err := m.embeddings.GenerateEmbedding(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to generate query embedding: %w", err)
	}

	results, err := m.collection.QueryEmbedding(ctx, embedding, topK, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to search memories: %w", err)
	}

	memoryResults := make([]MemorySearchResult, 0, len(results))
	for _, result := range results {
		isEncrypted := getMetadataString(result.Metadata, "is_encrypted") == "true"

		metadata := MemoryMetadata{
			Source:      getMetadataString(result.Metadata, "source"),
			SessionID:   getMetadataString(result.Metadata, "session_id"),
			User:        getMetadataString(result.Metadata, "user"),
			Tags:        strings.Split(getMetadataString(result.Metadata, "tags"), ","),
			IsEncrypted: isEncrypted,
		}

		createdAt, _ := time.Parse(time.RFC3339, getMetadataString(result.Metadata, "created_at"))
		updatedAt, _ := time.Parse(time.RFC3339, getMetadataString(result.Metadata, "updated_at"))

		memory := Memory{
			ID:         result.ID,
			Content:    result.Content,
			Metadata:   metadata,
			CreatedAt:  createdAt,
			UpdatedAt:  updatedAt,
			Importance: float32(getMetadataFloat64(result.Metadata, "importance")),
		}

		memoryResults = append(memoryResults, MemorySearchResult{
			Memory:     memory,
			Similarity: result.Similarity,
		})
	}

	return memoryResults, nil
}

func (m *MemoryManager) GetMemory(ctx context.Context, id string) (*Memory, error) {
	if !m.initialized {
		return nil, fmt.Errorf("memory manager not initialized")
	}

	result, err := m.collection.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get memory: %w", err)
	}

	metadata := MemoryMetadata{
		Source:      getMetadataString(result.Metadata, "source"),
		SessionID:   getMetadataString(result.Metadata, "session_id"),
		User:        getMetadataString(result.Metadata, "user"),
		Tags:        strings.Split(getMetadataString(result.Metadata, "tags"), ","),
		IsEncrypted: getMetadataString(result.Metadata, "is_encrypted") == "true",
	}

	createdAt, _ := time.Parse(time.RFC3339, getMetadataString(result.Metadata, "created_at"))
	updatedAt, _ := time.Parse(time.RFC3339, getMetadataString(result.Metadata, "updated_at"))

	return &Memory{
		ID:         result.ID,
		Content:    result.Content,
		Metadata:   metadata,
		CreatedAt:  createdAt,
		UpdatedAt:  updatedAt,
		Importance: float32(getMetadataFloat64(result.Metadata, "importance")),
	}, nil
}

func (m *MemoryManager) DeleteMemory(ctx context.Context, id string) error {
	if !m.initialized {
		return fmt.Errorf("memory manager not initialized")
	}

	if err := m.collection.Delete(ctx, nil, nil, id); err != nil {
		return fmt.Errorf("failed to delete memory: %w", err)
	}

	return nil
}

func (m *MemoryManager) UpdateMemoryImportance(ctx context.Context, id string, importance float32) error {
	if !m.initialized {
		return fmt.Errorf("memory manager not initialized")
	}

	memory, err := m.GetMemory(ctx, id)
	if err != nil {
		return err
	}

	memory.Importance = importance
	memory.UpdatedAt = time.Now()

	docMetadata := map[string]string{
		"created_at":   memory.CreatedAt.Format(time.RFC3339),
		"updated_at":   memory.UpdatedAt.Format(time.RFC3339),
		"importance":   fmt.Sprintf("%f", memory.Importance),
		"source":       memory.Metadata.Source,
		"session_id":   memory.Metadata.SessionID,
		"user":         memory.Metadata.User,
		"tags":         strings.Join(memory.Metadata.Tags, ","),
		"is_encrypted": fmt.Sprintf("%v", memory.Metadata.IsEncrypted),
	}

	doc, err := chromem.NewDocument(ctx, memory.ID, docMetadata, nil, memory.Content, nil)
	if err != nil {
		return fmt.Errorf("failed to create document: %w", err)
	}

	if err := m.collection.AddDocument(ctx, doc); err != nil {
		return fmt.Errorf("failed to update memory: %w", err)
	}

	return nil
}

func (m *MemoryManager) GetAllMemories(ctx context.Context) ([]Memory, error) {
	if !m.initialized {
		return nil, fmt.Errorf("memory manager not initialized")
	}

	count := m.collection.Count()
	if count == 0 {
		return []Memory{}, nil
	}

	results, err := m.collection.Query(ctx, "memory", count, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get all memories: %w", err)
	}

	memories := make([]Memory, 0, len(results))
	for _, result := range results {
		isEncrypted := getMetadataString(result.Metadata, "is_encrypted") == "true"

		metadata := MemoryMetadata{
			Source:      getMetadataString(result.Metadata, "source"),
			SessionID:   getMetadataString(result.Metadata, "session_id"),
			User:        getMetadataString(result.Metadata, "user"),
			Tags:        strings.Split(getMetadataString(result.Metadata, "tags"), ","),
			IsEncrypted: isEncrypted,
		}

		createdAt, _ := time.Parse(time.RFC3339, getMetadataString(result.Metadata, "created_at"))
		updatedAt, _ := time.Parse(time.RFC3339, getMetadataString(result.Metadata, "updated_at"))

		memories = append(memories, Memory{
			ID:         result.ID,
			Content:    result.Content,
			Metadata:   metadata,
			CreatedAt:  createdAt,
			UpdatedAt:  updatedAt,
			Importance: float32(getMetadataFloat64(result.Metadata, "importance")),
		})
	}

	return memories, nil
}

func (m *MemoryManager) ConsolidateMemories(ctx context.Context) (int, error) {
	if !m.initialized {
		return 0, fmt.Errorf("memory manager not initialized")
	}

	memories, err := m.GetAllMemories(ctx)
	if err != nil {
		return 0, err
	}

	consolidated := 0
	threshold := time.Now().AddDate(0, -3, 0)
	lowImportanceThreshold := float32(0.2)

	for _, memory := range memories {
		if memory.Importance < lowImportanceThreshold {
			if err := m.DeleteMemory(ctx, memory.ID); err != nil {
				continue
			}
			consolidated++
		} else if memory.CreatedAt.Before(threshold) && memory.Importance < 0.5 {
			if err := m.DeleteMemory(ctx, memory.ID); err != nil {
				continue
			}
			consolidated++
		}
	}

	return consolidated, nil
}

func getMetadataString(metadata map[string]string, key string) string {
	if val, ok := metadata[key]; ok {
		return val
	}
	return ""
}

func getMetadataFloat64(metadata map[string]string, key string) float64 {
	if val, ok := metadata[key]; ok {
		var f float64
		fmt.Sscanf(val, "%f", &f)
		return f
	}
	return 0
}

func generateUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
