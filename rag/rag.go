package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/philippgille/chromem-go"
)

const (
	configDir            = ".config/terminal-ai"
	MemoryDBFileName     = "memory.db"
	MemoryCollectionName = "memories"
)

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
			ChunkSize:     1000,
			ChunkOverlap:  200,
			VectorWeight:  0.6,
			KeywordWeight: 0.4,
		},
		dataDir:     dataDir,
		db:          db,
		collection:  collection,
		incremental: incremental,
		chunker:     NewChunker(),
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

func (m *RAGManager) IndexDirectories(dirs []string) (*UpdateReport, error) {
	if m.incremental == nil {
		return nil, fmt.Errorf("incremental updater not initialized")
	}

	report, err := m.incremental.UpdateIndex(dirs, func(chunks []Chunk, sourcePath string) error {
		return m.indexChunks(chunks)
	})
	if err != nil {
		return nil, err
	}

	return report, nil
}

func (m *RAGManager) indexChunks(chunks []Chunk) error {
	ctx := context.Background()

	for _, chunk := range chunks {
		docMetadata := map[string]string{
			"source_path":  chunk.SourcePath,
			"source_type":  chunk.SourceType,
			"chunk_index":  fmt.Sprintf("%d", chunk.ChunkIndex),
			"total_chunks": fmt.Sprintf("%d", chunk.TotalChunks),
			"created_at":   chunk.CreatedAt.Format(time.RFC3339),
			"content_hash": chunk.ID,
		}

		for key, val := range chunk.Metadata {
			docMetadata[key] = val
		}

		doc, err := chromem.NewDocument(ctx, chunk.ID, docMetadata, nil, chunk.Content, nil)
		if err != nil {
			continue
		}

		if err := m.collection.AddDocument(ctx, doc); err != nil {
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

	return m.indexChunks(chunks)
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

		m.indexChunks(chunks)
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
