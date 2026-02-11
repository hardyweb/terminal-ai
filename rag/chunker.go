package rag

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type ChunkerConfig struct {
	ChunkSize     int
	ChunkOverlap  int
	MinChunkSize  int
	SplitStrategy string
}

type Chunk struct {
	ID          string            `json:"id"`
	Content     string            `json:"content"`
	SourcePath  string            `json:"source_path"`
	SourceURL   string            `json:"source_url,omitempty"`
	SourceType  string            `json:"source_type"`
	ChunkIndex  int               `json:"chunk_index"`
	TotalChunks int               `json:"total_chunks"`
	Metadata    map[string]string `json:"metadata"`
	CreatedAt   time.Time         `json:"created_at"`
}

type Chunker struct {
	config ChunkerConfig
}

func NewChunker() *Chunker {
	return &Chunker{
		config: ChunkerConfig{
			ChunkSize:     500,
			ChunkOverlap:  50,
			MinChunkSize:  50,
			SplitStrategy: "paragraph",
		},
	}
}

func NewChunkerWithConfig(config ChunkerConfig) *Chunker {
	if config.ChunkSize == 0 {
		config.ChunkSize = 1000
	}
	if config.ChunkOverlap == 0 {
		config.ChunkOverlap = 200
	}
	if config.MinChunkSize == 0 {
		config.MinChunkSize = 100
	}
	if config.SplitStrategy == "" {
		config.SplitStrategy = "paragraph"
	}
	return &Chunker{config: config}
}

func (c *Chunker) ChunkDocument(content, sourcePath string) ([]Chunk, error) {
	return c.ChunkWithOverlap(content, sourcePath)
}

func (c *Chunker) ChunkWithOverlap(content, sourcePath string) ([]Chunk, error) {
	paragraphs := c.splitByParagraphs(content)
	var chunks []Chunk

	for i := 0; i < len(paragraphs); i++ {
		paragraph := paragraphs[i]

		if len(paragraph) < c.config.MinChunkSize && i < len(paragraphs)-1 {
			paragraph += "\n\n" + paragraphs[i+1]
			paragraph = strings.TrimSpace(paragraph)
		}

		if len(paragraph) <= c.config.ChunkSize {
			chunk := c.createChunk(paragraph, sourcePath, len(chunks), len(paragraphs))
			chunk.Metadata["chunking_strategy"] = "single_paragraph"
			chunks = append(chunks, chunk)
			continue
		}

		subChunks := c.splitLongContent(paragraph, sourcePath, len(chunks), len(paragraphs))
		chunks = append(chunks, subChunks...)
	}

	chunks = c.applyOverlap(chunks, sourcePath)
	return chunks, nil
}

func (c *Chunker) SmartChunkByParagraphs(content, sourcePath string) ([]Chunk, error) {
	return c.ChunkWithOverlap(content, sourcePath)
}

func (c *Chunker) splitByParagraphs(content string) []string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")

	paragraphPattern := regexp.MustCompile(`\n\s*\n`)
	paragraphs := paragraphPattern.Split(content, -1)

	var result []string
	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		if len(p) > 0 {
			result = append(result, p)
		}
	}

	return result
}

func (c *Chunker) splitLongContent(content, sourcePath string, startIndex, totalParagraphs int) []Chunk {
	var chunks []Chunk
	remaining := content
	currentIndex := startIndex

	for len(remaining) > 0 {
		chunkSize := c.config.ChunkSize
		if len(remaining) <= chunkSize {
			chunkSize = len(remaining)
		}

		chunkContent := remaining[:chunkSize]

		lastNewline := strings.LastIndex(chunkContent, "\n")
		lastSpace := strings.LastIndex(chunkContent, " ")

		cutPoint := chunkSize
		if lastNewline > chunkSize/2 {
			cutPoint = lastNewline + 1
		} else if lastSpace > chunkSize/2 {
			cutPoint = lastSpace
		}

		chunkContent = strings.TrimSpace(remaining[:cutPoint])
		remaining = strings.TrimSpace(remaining[cutPoint:])

		if len(chunkContent) < c.config.MinChunkSize && len(remaining) > 0 {
			if len(remaining) < c.config.MinChunkSize {
				chunkContent = chunkContent + "\n\n" + remaining
				remaining = ""
			} else {
				continue
			}
		}

		chunk := c.createChunk(chunkContent, sourcePath, currentIndex, totalParagraphs)
		chunk.Metadata["chunking_strategy"] = "split_long_paragraph"
		chunks = append(chunks, chunk)
		currentIndex++
	}

	return chunks
}

func (c *Chunker) applyOverlap(chunks []Chunk, sourcePath string) []Chunk {
	if len(chunks) < 2 || c.config.ChunkOverlap <= 0 {
		return chunks
	}

	var result []Chunk
	for i := range chunks {
		if i > 0 {
			prevChunk := result[len(result)-1]
			overlapContent := prevChunk.Content

			if len(overlapContent) > c.config.ChunkOverlap {
				overlapContent = overlapContent[len(overlapContent)-c.config.ChunkOverlap:]
			}

			overlapContent = "\n[...continued from previous chunk...]\n" + overlapContent
			chunks[i].Content = overlapContent + chunks[i].Content
			chunks[i].Metadata["has_overlap"] = "true"
			chunks[i].Metadata["overlap_size"] = fmt.Sprintf("%d", len(overlapContent))
		}
		result = append(result, chunks[i])
	}

	return result
}

func (c *Chunker) createChunk(content, sourcePath string, chunkIndex, totalChunks int) Chunk {
	contentHash := c.hashContent(content)

	return Chunk{
		ID:          contentHash[:16],
		Content:     content,
		SourcePath:  sourcePath,
		SourceType:  "file",
		ChunkIndex:  chunkIndex,
		TotalChunks: totalChunks,
		Metadata: map[string]string{
			"char_count":    fmt.Sprintf("%d", len(content)),
			"word_count":    fmt.Sprintf("%d", c.countWords(content)),
			"chunking_mode": "smart",
		},
		CreatedAt: time.Now(),
	}
}

func (c *Chunker) ExtractMetadata(content, sourcePath string) map[string]string {
	metadata := make(map[string]string)

	title := c.extractTitle(content)
	if title != "" {
		metadata["title"] = title
	}

	headers := c.extractHeaders(content)
	if len(headers) > 0 {
		metadata["headers"] = strings.Join(headers, "|")
	}

	language := c.detectLanguage(content)
	metadata["language"] = language

	return metadata
}

func (c *Chunker) extractTitle(content string) string {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if len(line) > 0 && len(line) < 200 {
			if strings.HasPrefix(line, "# ") {
				return strings.TrimPrefix(line, "# ")
			}
			if len(line) < 100 && !strings.Contains(line, " ") {
				continue
			}
			return line
		}
	}
	return ""
}

func (c *Chunker) extractHeaders(content string) []string {
	headerPattern := regexp.MustCompile(`(?m)^#{1,6}\s+.+$`)
	matches := headerPattern.FindAllString(content, -1)

	var headers []string
	for _, match := range matches {
		level := strings.Count(match, "#")
		text := strings.TrimSpace(strings.TrimPrefix(match, strings.Repeat("#", level)))
		headers = append(headers, fmt.Sprintf("h%d:%s", level, text))
	}

	return headers
}

func (c *Chunker) detectLanguage(content string) string {
	content = strings.ToLower(content)

	indicators := map[string]string{
		"func ":     "go",
		"funciton ": "python",
		"def ":      "python",
		"class ":    "java",
		"public ":   "java",
		"import ":   "java",
		"<?php":     "php",
		"function ": "javascript",
		"const ":    "javascript",
		"let ":      "javascript",
		"SELECT ":   "sql",
		"FROM ":     "sql",
	}

	for indicator, lang := range indicators {
		if strings.Contains(content, indicator) {
			return lang
		}
	}

	return "unknown"
}

func (c *Chunker) countWords(content string) int {
	wordPattern := regexp.MustCompile(`\b\w+\b`)
	return len(wordPattern.FindAllString(content, -1))
}

func (c *Chunker) hashContent(content string) string {
	hash := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hash[:])
}

func (c *Chunker) ChunkWebContent(content, url string) ([]Chunk, error) {
	title := c.extractTitle(content)
	headers := c.extractHeaders(content)

	baseName := filepath.Base(url)
	if len(baseName) > 50 {
		baseName = baseName[:50]
	}

	sourcePath := fmt.Sprintf("web:%s", baseName)

	chunks, err := c.ChunkWithOverlap(content, sourcePath)
	if err != nil {
		return nil, err
	}

	for i := range chunks {
		chunks[i].SourceType = "web"
		chunks[i].SourceURL = url
		chunks[i].Metadata["web_title"] = title
		if len(headers) > 0 {
			chunks[i].Metadata["section_headers"] = strings.Join(headers[:5], " > ")
		}
	}

	return chunks, nil
}

func EnsureChunksDir(dataDir string) error {
	chunksDir := filepath.Join(dataDir, "rag", "chunks")
	return os.MkdirAll(chunksDir, 0755)
}

func SaveChunk(dataDir string, chunk Chunk) error {
	chunksDir := filepath.Join(dataDir, "rag", "chunks")
	if err := os.MkdirAll(chunksDir, 0755); err != nil {
		return err
	}

	chunkFile := filepath.Join(chunksDir, fmt.Sprintf("%s.json", chunk.ID))
	data, err := json.MarshalIndent(chunk, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(chunkFile, data, 0644)
}

func LoadChunk(dataDir, chunkID string) (*Chunk, error) {
	chunkFile := filepath.Join(dataDir, "rag", "chunks", fmt.Sprintf("%s.json", chunkID))
	data, err := os.ReadFile(chunkFile)
	if err != nil {
		return nil, err
	}

	var chunk Chunk
	if err := json.Unmarshal(data, &chunk); err != nil {
		return nil, err
	}

	return &chunk, nil
}

func LoadAllChunks(dataDir string) ([]Chunk, error) {
	chunksDir := filepath.Join(dataDir, "rag", "chunks")
	entries, err := os.ReadDir(chunksDir)
	if err != nil {
		return nil, err
	}

	var chunks []Chunk
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		chunk, err := LoadChunk(dataDir, strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			continue
		}
		chunks = append(chunks, *chunk)
	}

	return chunks, nil
}

func DeleteChunk(dataDir, chunkID string) error {
	chunkFile := filepath.Join(dataDir, "rag", "chunks", fmt.Sprintf("%s.json", chunkID))
	return os.Remove(chunkFile)
}

func DeleteChunksBySource(dataDir, sourcePath string) error {
	chunks, err := LoadAllChunks(dataDir)
	if err != nil {
		return err
	}

	for _, chunk := range chunks {
		if chunk.SourcePath == sourcePath {
			if err := DeleteChunk(dataDir, chunk.ID); err != nil {
				continue
			}
		}
	}

	return nil
}
