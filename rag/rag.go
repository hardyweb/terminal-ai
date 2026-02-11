package rag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/philippgille/chromem-go"
)

const (
	configDir            = ".config/terminal-ai"
	MemoryDBFileName     = "memory.db"
	MemoryCollectionName = "memories"
)

const (
	OpenRouterEmbeddingsURL = "https://openrouter.ai/api/v1/embeddings"
	OllamaEmbeddingsURL     = "http://localhost:11434/api/embeddings"
)

type EmbeddingService struct {
	apiURL    string
	model     string
	timeout   time.Duration
	useOllama bool
}

func NewEmbeddingService() *EmbeddingService {
	useOllama := os.Getenv("USE_OLLAMA_EMBEDDINGS") == "true"
	ollamaURL := os.Getenv("OLLAMA_EMBEDDINGS_URL")

	if useOllama && ollamaURL != "" {
		return &EmbeddingService{
			apiURL:    ollamaURL,
			model:     os.Getenv("OLLAMA_EMBEDDINGS_MODEL"),
			timeout:   30 * time.Minute,
			useOllama: true,
		}
	}

	return &EmbeddingService{
		apiURL:    OpenRouterEmbeddingsURL,
		model:     "text-embedding-3-small",
		timeout:   60 * time.Second,
		useOllama: false,
	}
}

func (e *EmbeddingService) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
	if e.useOllama {
		return e.generateOllamaEmbedding(ctx, text)
	}
	return e.generateOpenRouterEmbedding(ctx, text)
}

func (e *EmbeddingService) generateOllamaEmbedding(ctx context.Context, text string) ([]float32, error) {
	payload := map[string]interface{}{
		"model":  e.model,
		"prompt": text,
		"stream": false,
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

	client := &http.Client{Timeout: e.timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call Ollama embedding API: %w", err)
	}
	defer resp.Body.Close()

	bodyResp, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Ollama embedding API returned status %d: %s", resp.StatusCode, string(bodyResp))
	}

	var result struct {
		Embedding []float32 `json:"embedding"`
	}
	if err := json.Unmarshal(bodyResp, &result); err != nil {
		return nil, fmt.Errorf("failed to decode embeddings: %w", err)
	}

	if len(result.Embedding) == 0 {
		return nil, fmt.Errorf("no embeddings returned from Ollama")
	}

	return result.Embedding, nil
}

func (e *EmbeddingService) generateOpenRouterEmbedding(ctx context.Context, text string) ([]float32, error) {
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

type RAGConfig struct {
	DataDir       string
	ChunkSize     int
	ChunkOverlap  int
	VectorWeight  float64
	KeywordWeight float64
}

type SearchResult struct {
	Chunk       Chunk
	Content     string
	SourcePath  string
	SourceURL   string
	SourceType  string
	Similarity  float64
	ChunkIndex  int
	TotalChunks int
}

type RAGManager struct {
	config      *RAGConfig
	dataDir     string
	db          *chromem.DB
	collection  *chromem.Collection
	incremental *IncrementalUpdater
	chunker     *Chunker
	embeddings  *EmbeddingService
	initialized bool
}

type WebCacheEntry struct {
	URL         string    `json:"url"`
	Content     string    `json:"content"`
	Title       string    `json:"title"`
	IndexedAt   time.Time `json:"indexed_at"`
	ContentHash string    `json:"content_hash"`
}

func NewRAGManager() (*RAGManager, error) {
	homeDir, _ := os.UserHomeDir()
	dataDir := filepath.Join(homeDir, ".local", "share", "terminal-ai")

	return NewRAGManagerWithDataDir(dataDir)
}

func NewRAGManagerWithDataDir(dataDir string) (*RAGManager, error) {
	ragDir := filepath.Join(dataDir, "rag")
	if err := os.MkdirAll(ragDir, 0755); err != nil {
		return nil, err
	}

	chunksDir := filepath.Join(ragDir, "chunks")
	if err := os.MkdirAll(chunksDir, 0755); err != nil {
		return nil, err
	}

	webCacheDir := filepath.Join(ragDir, "web-cache")
	if err := os.MkdirAll(webCacheDir, 0755); err != nil {
		return nil, err
	}

	incremental, err := NewIncrementalUpdater(dataDir)
	if err != nil {
		return nil, err
	}

	dbPath := filepath.Join(dataDir, MemoryDBFileName)

	db, err := chromem.NewPersistentDB(dbPath, false)
	if err != nil {
		return nil, err
	}

	collection, err := db.GetOrCreateCollection(MemoryCollectionName, nil, nil)
	if err != nil {
		db.Reset()
		return nil, err
	}

	mgr := &RAGManager{
		config: &RAGConfig{
			DataDir:       dataDir,
			ChunkSize:     500,
			ChunkOverlap:  50,
			VectorWeight:  0.6,
			KeywordWeight: 0.4,
		},
		dataDir:     dataDir,
		db:          db,
		collection:  collection,
		incremental: incremental,
		chunker:     NewChunker(),
		embeddings:  NewEmbeddingService(),
		initialized: true,
	}

	return mgr, nil
}

func (m *RAGManager) Close() error {
	if m.db != nil {
		return m.db.Reset()
	}
	return nil
}

func (m *RAGManager) GetDataDir() string {
	return m.dataDir
}

func (m *RAGManager) IndexDirectory(dir string) (*UpdateReport, error) {
	return m.IndexDirectories([]string{dir})
}

type ChunkProgressCallback func(current int, total int)

func (m *RAGManager) IndexDirectories(dirs []string) (*UpdateReport, error) {
	return m.IndexDirectoriesWithProgress(dirs, nil)
}

func (m *RAGManager) IndexDirectoriesWithProgress(dirs []string, chunkProgress ChunkProgressCallback) (*UpdateReport, error) {
	if m.incremental == nil {
		return nil, fmt.Errorf("incremental updater not initialized")
	}

	report, err := m.incremental.UpdateIndex(dirs, func(chunks []Chunk, sourcePath string) error {
		return m.indexChunks(chunks, chunkProgress)
	})
	if err != nil {
		return nil, err
	}

	return report, nil
}

func (m *RAGManager) indexChunks(chunks []Chunk, progress ChunkProgressCallback) error {
	ctx := context.Background()

	if m.embeddings == nil {
		m.embeddings = NewEmbeddingService()
	}

	type result struct {
		chunk     Chunk
		embedding []float32
		err       error
	}

	results := make(chan result, len(chunks))
	chunkChan := make(chan Chunk, len(chunks))
	var wg sync.WaitGroup

	// Send all chunks to the channel
	for _, chunk := range chunks {
		chunkChan <- chunk
	}
	close(chunkChan)

	numWorkers := 1
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for chunk := range chunkChan {
				embedding, err := m.embeddings.GenerateEmbedding(ctx, chunk.Content)
				if err != nil {
					results <- result{err: err}
				} else {
					results <- result{chunk: chunk, embedding: embedding}
				}
			}
		}(i)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	successCount := 0
	errorCount := 0
	resultCount := 0
	totalChunks := len(chunks)
	for res := range results {
		resultCount++
		if res.err != nil {
			errorCount++
		} else {
			successCount++
		}

		// Call progress callback every chunk
		if progress != nil {
			progress(resultCount, totalChunks)
		}

		if res.err != nil {
			continue
		}

		docMetadata := map[string]string{
			"source_path":  res.chunk.SourcePath,
			"source_type":  res.chunk.SourceType,
			"chunk_index":  fmt.Sprintf("%d", res.chunk.ChunkIndex),
			"total_chunks": fmt.Sprintf("%d", res.chunk.TotalChunks),
			"created_at":   res.chunk.CreatedAt.Format(time.RFC3339),
			"content_hash": res.chunk.ID,
		}

		for key, val := range res.chunk.Metadata {
			docMetadata[key] = val
		}

		doc, err := chromem.NewDocument(ctx, res.chunk.ID, docMetadata, res.embedding, res.chunk.Content, nil)
		if err != nil {
			errorCount++
			continue
		}

		if err := m.collection.AddDocument(ctx, doc); err != nil {
			errorCount++
			continue
		}
	}

	return nil
}

func (m *RAGManager) AddSource(sourcePath string) error {
	chunks, err := ChunkDocumentFromPath(sourcePath)
	if err != nil {
		return err
	}

	return m.indexChunks(chunks, nil)
}

func (m *RAGManager) RemoveSource(sourcePath string) error {
	if m.incremental != nil {
		return m.incremental.RemoveSource(sourcePath)
	}

	chunkIDs, err := m.incremental.GetChunkIDsForSource(sourcePath)
	if err != nil {
		return err
	}

	ctx := context.Background()
	for _, chunkID := range chunkIDs {
		m.collection.Delete(ctx, nil, nil, chunkID)
		DeleteChunk(m.dataDir, chunkID)
	}

	return nil
}

func (m *RAGManager) Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 10
	}

	count := m.collection.Count()
	if limit > int(count) {
		limit = int(count)
	}

	if limit == 0 {
		return []SearchResult{}, nil
	}

	if m.embeddings == nil {
		m.embeddings = NewEmbeddingService()
	}

	embedding, err := m.embeddings.GenerateEmbedding(ctx, query)
	if err != nil {
		results, err := m.collection.Query(ctx, query, limit, nil, nil)
		if err != nil {
			return nil, err
		}

		var searchResults []SearchResult
		for _, result := range results {
			chunk, err := LoadChunk(m.dataDir, result.ID)
			if err != nil {
				continue
			}

			searchResults = append(searchResults, SearchResult{
				Chunk:       *chunk,
				Content:     chunk.Content,
				SourcePath:  chunk.SourcePath,
				SourceURL:   chunk.SourceURL,
				SourceType:  chunk.SourceType,
				Similarity:  float64(result.Similarity),
				ChunkIndex:  chunk.ChunkIndex,
				TotalChunks: chunk.TotalChunks,
			})
		}

		return searchResults, nil
	}

	results, err := m.collection.QueryEmbedding(ctx, embedding, limit, nil, nil)
	if err != nil {
		results, err = m.collection.Query(ctx, query, limit, nil, nil)
		if err != nil {
			return nil, err
		}
	}

	var searchResults []SearchResult
	for _, result := range results {
		chunk, err := LoadChunk(m.dataDir, result.ID)
		if err != nil {
			continue
		}

		searchResults = append(searchResults, SearchResult{
			Chunk:       *chunk,
			Content:     chunk.Content,
			SourcePath:  chunk.SourcePath,
			SourceURL:   chunk.SourceURL,
			SourceType:  chunk.SourceType,
			Similarity:  float64(result.Similarity),
			ChunkIndex:  chunk.ChunkIndex,
			TotalChunks: chunk.TotalChunks,
		})
	}

	return searchResults, nil
}

func (m *RAGManager) GetStats() (*IndexStats, error) {
	if m.incremental != nil {
		return m.incremental.GetStats()
	}

	return &IndexStats{
		TotalSources: 0,
		TotalChunks:  0,
		TotalSize:    0,
		LastUpdated:  time.Now(),
	}, nil
}

func (m *RAGManager) ListSources() ([]SourceInfo, error) {
	if m.incremental != nil {
		return m.incremental.ListSources()
	}
	return []SourceInfo{}, nil
}

func (m *RAGManager) ClearAll() error {
	if m.incremental != nil {
		if err := m.incremental.ClearAll(); err != nil {
			return err
		}
	}

	ctx := context.Background()
	if m.collection != nil {
		m.collection.Delete(ctx, nil, nil)
	}

	chunksDir := filepath.Join(m.dataDir, "rag", "chunks")
	os.RemoveAll(chunksDir)
	os.MkdirAll(chunksDir, 0755)

	return nil
}

func (m *RAGManager) GetWebCacheDir() string {
	return filepath.Join(m.dataDir, "rag", "web-cache")
}

func (m *RAGManager) CacheWebContent(entry WebCacheEntry) error {
	cacheDir := m.GetWebCacheDir()
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return err
	}

	hash := m.incremental.CalculateContentHash(entry.Content)

	cacheFile := filepath.Join(cacheDir, fmt.Sprintf("%s.json", hash[:16]))
	entry.ContentHash = hash

	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(cacheFile, data, 0644)
}

func (m *RAGManager) GetWebCachedContent(contentHash string) (*WebCacheEntry, error) {
	cacheDir := m.GetWebCacheDir()
	cacheFile := filepath.Join(cacheDir, fmt.Sprintf("%s.json", contentHash[:16]))

	data, err := os.ReadFile(cacheFile)
	if err != nil {
		return nil, err
	}

	var entry WebCacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, err
	}

	return &entry, nil
}

func ChunkDocumentFromPath(sourcePath string) ([]Chunk, error) {
	content, err := os.ReadFile(sourcePath)
	if err != nil {
		return nil, err
	}

	chunker := NewChunker()
	return chunker.ChunkWithOverlap(string(content), sourcePath)
}

type LegacyRAGIndex struct {
	Documents []LegacyRAGDocument `json:"documents"`
}

type LegacyRAGDocument struct {
	Path       string   `json:"path"`
	Content    string   `json:"content"`
	Keywords   []string `json:"keywords"`
	IndexedAt  string   `json:"indexed_at"`
	Owner      string   `json:"owner"`
	Visibility string   `json:"visibility"`
}

func (m *RAGManager) MigrateLegacyIndex() error {
	oldIndexFile := filepath.Join(m.dataDir, "rag-index.json")

	data, err := os.ReadFile(oldIndexFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var legacyIndex LegacyRAGIndex
	if err := json.Unmarshal(data, &legacyIndex); err != nil {
		return err
	}

	for _, doc := range legacyIndex.Documents {
		chunks, err := m.chunker.ChunkWithOverlap(doc.Content, doc.Path)
		if err != nil {
			continue
		}

		m.indexChunks(chunks, nil)
	}

	return nil
}

func GetRAGDataDir() string {
	if xdgData := os.Getenv("XDG_DATA_HOME"); xdgData != "" {
		return filepath.Join(xdgData, "terminal-ai")
	}
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".local", "share", "terminal-ai")
}
