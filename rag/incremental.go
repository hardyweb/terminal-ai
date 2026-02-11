package rag

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type SourceManifest struct {
	Sources     map[string]SourceInfo `json:"sources"`
	LastUpdated time.Time             `json:"last_updated"`
	Version     int                   `json:"version"`
}

type SourceInfo struct {
	Path         string    `json:"path"`
	FileHash     string    `json:"file_hash"`
	FileSize     int64     `json:"file_size"`
	LastModified time.Time `json:"last_modified"`
	LastIndexed  time.Time `json:"last_indexed"`
	ChunkIDs     []string  `json:"chunk_ids"`
	SourceType   string    `json:"source_type"`
	URL          string    `json:"url,omitempty"`
	Status       string    `json:"status"`
	ErrorMsg     string    `json:"error_msg,omitempty"`
}

type UpdateReport struct {
	Added       int            `json:"added"`
	Updated     int            `json:"updated"`
	Unchanged   int            `json:"unchanged"`
	Errors      int            `json:"errors"`
	TotalChunks int            `json:"total_chunks"`
	Details     []SourceDetail `json:"details"`
	Duration    time.Duration  `json:"duration"`
}

type SourceDetail struct {
	Path   string `json:"path"`
	Action string `json:"action"`
	Chunks int    `json:"chunks"`
	Error  string `json:"error,omitempty"`
}

type IncrementalUpdater struct {
	manifestPath string
	dataDir      string
	chunker      *Chunker
}

func NewIncrementalUpdater(dataDir string) (*IncrementalUpdater, error) {
	manifestPath := filepath.Join(dataDir, "rag", "manifest.json")

	updater := &IncrementalUpdater{
		manifestPath: manifestPath,
		dataDir:      dataDir,
		chunker:      NewChunker(),
	}

	if err := os.MkdirAll(filepath.Dir(manifestPath), 0755); err != nil {
		return nil, err
	}

	return updater, nil
}

func (u *IncrementalUpdater) LoadManifest() (*SourceManifest, error) {
	data, err := os.ReadFile(u.manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &SourceManifest{
				Sources: make(map[string]SourceInfo),
			}, nil
		}
		return nil, err
	}

	var manifest SourceManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}

	if manifest.Sources == nil {
		manifest.Sources = make(map[string]SourceInfo)
	}

	return &manifest, nil
}

func (u *IncrementalUpdater) SaveManifest(manifest *SourceManifest) error {
	manifest.LastUpdated = time.Now()
	manifest.Version++

	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(u.manifestPath, data, 0644)
}

func (u *IncrementalUpdater) CalculateFileHash(path string) (string, int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", 0, err
	}

	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		return "", 0, err
	}

	hash := sha256.Sum256(content)
	return hex.EncodeToString(hash[:]), info.Size(), nil
}

func (u *IncrementalUpdater) CalculateContentHash(content string) string {
	hash := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hash[:])
}

func (u *IncrementalUpdater) ScanDirectory(dir string) ([]string, error) {
	var files []string

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		ext := filepath.Ext(path)
		lowerExt := ""
		if ext != "" {
			lowerExt = strings.ToLower(ext[1:])
		}

		if lowerExt == "txt" || lowerExt == "md" || lowerExt == "json" || lowerExt == "yaml" || lowerExt == "yml" {
			files = append(files, path)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return files, nil
}

func (u *IncrementalUpdater) GetChangedSources(dir string, manifest *SourceManifest) ([]SourceInfo, []SourceInfo, []SourceInfo, error) {
	files, err := u.ScanDirectory(dir)
	if err != nil {
		return nil, nil, nil, err
	}

	var newSources []SourceInfo
	var changedSources []SourceInfo
	var unchangedSources []SourceInfo

	for _, filePath := range files {
		info, err := os.Stat(filePath)
		if err != nil {
			continue
		}

		fileHash, fileSize, err := u.CalculateFileHash(filePath)
		if err != nil {
			continue
		}

		sourceInfo := SourceInfo{
			Path:         filePath,
			FileHash:     fileHash,
			FileSize:     fileSize,
			LastModified: info.ModTime(),
		}

		existing, exists := manifest.Sources[filePath]
		if !exists {
			sourceInfo.SourceType = "file"
			sourceInfo.Status = "pending"
			newSources = append(newSources, sourceInfo)
		} else if existing.FileHash != fileHash || existing.FileSize != fileSize {
			sourceInfo.ChunkIDs = existing.ChunkIDs
			sourceInfo.SourceType = existing.SourceType
			sourceInfo.Status = "pending"
			changedSources = append(changedSources, sourceInfo)
		} else {
			sourceInfo.ChunkIDs = existing.ChunkIDs
			sourceInfo.SourceType = existing.SourceType
			sourceInfo.LastIndexed = existing.LastIndexed
			sourceInfo.Status = existing.Status
			unchangedSources = append(unchangedSources, sourceInfo)
		}
	}

	return newSources, changedSources, unchangedSources, nil
}

func (u *IncrementalUpdater) ProcessFile(filePath string) ([]Chunk, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	chunks, err := u.chunker.ChunkWithOverlap(string(content), filePath)
	if err != nil {
		return nil, err
	}

	return chunks, nil
}

type ProgressCallback func(sourcePath string, current int, total int)

func (u *IncrementalUpdater) UpdateIndex(dirs []string, indexer func([]Chunk, string) error) (*UpdateReport, error) {
	return u.UpdateIndexWithProgress(dirs, indexer, nil)
}

func (u *IncrementalUpdater) UpdateIndexWithProgress(dirs []string, indexer func([]Chunk, string) error, progress ProgressCallback) (*UpdateReport, error) {
	startTime := time.Now()

	manifest, err := u.LoadManifest()
	if err != nil {
		return nil, err
	}

	report := &UpdateReport{
		Details: make([]SourceDetail, 0),
	}

	allNewSources := make([]SourceInfo, 0)
	allChangedSources := make([]SourceInfo, 0)
	allUnchangedSources := make([]SourceInfo, 0)

	for _, dir := range dirs {
		newSources, changedSources, unchangedSources, err := u.GetChangedSources(dir, manifest)
		if err != nil {
			return nil, err
		}

		allNewSources = append(allNewSources, newSources...)
		allChangedSources = append(allChangedSources, changedSources...)
		allUnchangedSources = append(allUnchangedSources, unchangedSources...)
	}

	totalToProcess := len(allNewSources) + len(allChangedSources)
	currentProcessed := 0

	for _, source := range allChangedSources {
		if len(source.ChunkIDs) > 0 {
			for _, chunkID := range source.ChunkIDs {
				DeleteChunk(u.dataDir, chunkID)
			}
		}
	}

	totalNewChunks := 0
	for _, source := range allNewSources {
		currentProcessed++
		if progress != nil {
			progress(source.Path, currentProcessed, totalToProcess)
		}

		chunks, err := u.ProcessFile(source.Path)
		if err != nil {
			report.Errors++
			report.Details = append(report.Details, SourceDetail{
				Path:   source.Path,
				Action: "error",
				Error:  err.Error(),
			})
			continue
		}

		if err := indexer(chunks, source.Path); err != nil {
			fmt.Fprintf(os.Stderr, "[ERROR] Failed to index chunks: %v\n", err)
		}

		var chunkIDs []string
		for _, chunk := range chunks {
			if err := SaveChunk(u.dataDir, chunk); err != nil {
				continue
			}
			chunkIDs = append(chunkIDs, chunk.ID)
			totalNewChunks++
		}

		source.ChunkIDs = chunkIDs
		source.Status = "indexed"
		source.LastIndexed = time.Now()
		manifest.Sources[source.Path] = source

		report.Added++
		report.Details = append(report.Details, SourceDetail{
			Path:   source.Path,
			Action: "added",
			Chunks: len(chunks),
		})
	}

	for _, source := range allChangedSources {
		currentProcessed++
		if progress != nil {
			progress(source.Path, currentProcessed, totalToProcess)
		}

		chunks, err := u.ProcessFile(source.Path)
		if err != nil {
			report.Errors++
			report.Details = append(report.Details, SourceDetail{
				Path:   source.Path,
				Action: "error",
				Error:  err.Error(),
			})
			continue
		}

		if err := indexer(chunks, source.Path); err != nil {
			fmt.Fprintf(os.Stderr, "[ERROR] Failed to index chunks: %v\n", err)
		}

		var chunkIDs []string
		for _, chunk := range chunks {
			if err := SaveChunk(u.dataDir, chunk); err != nil {
				continue
			}
			chunkIDs = append(chunkIDs, chunk.ID)
			totalNewChunks++
		}

		source.ChunkIDs = chunkIDs
		source.Status = "indexed"
		source.LastIndexed = time.Now()
		manifest.Sources[source.Path] = source

		report.Updated++
		report.Details = append(report.Details, SourceDetail{
			Path:   source.Path,
			Action: "updated",
			Chunks: len(chunks),
		})
	}

	for _, source := range allUnchangedSources {
		report.Unchanged++
		manifest.Sources[source.Path] = source
	}

	report.TotalChunks = totalNewChunks
	report.Duration = time.Since(startTime)

	if err := u.SaveManifest(manifest); err != nil {
		return nil, err
	}

	return report, nil
}

func (u *IncrementalUpdater) RemoveSource(sourcePath string) error {
	manifest, err := u.LoadManifest()
	if err != nil {
		return err
	}

	sourceInfo, exists := manifest.Sources[sourcePath]
	if !exists {
		return fmt.Errorf("source not found: %s", sourcePath)
	}

	for _, chunkID := range sourceInfo.ChunkIDs {
		DeleteChunk(u.dataDir, chunkID)
	}

	delete(manifest.Sources, sourcePath)

	return u.SaveManifest(manifest)
}

func (u *IncrementalUpdater) GetSourceStatus(sourcePath string) (*SourceInfo, error) {
	manifest, err := u.LoadManifest()
	if err != nil {
		return nil, err
	}

	sourceInfo, exists := manifest.Sources[sourcePath]
	if !exists {
		return nil, fmt.Errorf("source not found: %s", sourcePath)
	}

	return &sourceInfo, nil
}

func (u *IncrementalUpdater) ListSources() ([]SourceInfo, error) {
	manifest, err := u.LoadManifest()
	if err != nil {
		return nil, err
	}

	sources := make([]SourceInfo, 0, len(manifest.Sources))
	for _, info := range manifest.Sources {
		sources = append(sources, info)
	}

	return sources, nil
}

func (u *IncrementalUpdater) ClearAll() error {
	chunksDir := filepath.Join(u.dataDir, "rag", "chunks")
	if err := os.RemoveAll(chunksDir); err != nil {
		return err
	}

	if err := os.MkdirAll(chunksDir, 0755); err != nil {
		return err
	}

	manifest := &SourceManifest{
		Sources:     make(map[string]SourceInfo),
		LastUpdated: time.Now(),
		Version:     1,
	}

	return u.SaveManifest(manifest)
}

func (u *IncrementalUpdater) GetStats() (*IndexStats, error) {
	manifest, err := u.LoadManifest()
	if err != nil {
		return nil, err
	}

	stats := &IndexStats{
		TotalSources: len(manifest.Sources),
		TotalChunks:  0,
		Breakdown:    make(map[string]int),
	}

	totalSize := int64(0)
	for _, source := range manifest.Sources {
		stats.TotalChunks += len(source.ChunkIDs)
		stats.Breakdown[source.SourceType]++
		if source.SourceType == "file" {
			stat, err := os.Stat(source.Path)
			if err == nil {
				totalSize += stat.Size()
			}
		}
	}

	stats.TotalSize = totalSize
	stats.LastUpdated = manifest.LastUpdated

	return stats, nil
}

type IndexStats struct {
	TotalSources int            `json:"total_sources"`
	TotalChunks  int            `json:"total_chunks"`
	TotalSize    int64          `json:"total_size"`
	Breakdown    map[string]int `json:"breakdown"`
	LastUpdated  time.Time      `json:"last_updated"`
}

func FormatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}

	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func (u *IncrementalUpdater) GetChunkIDsForSource(sourcePath string) ([]string, error) {
	manifest, err := u.LoadManifest()
	if err != nil {
		return nil, err
	}

	sourceInfo, exists := manifest.Sources[sourcePath]
	if !exists {
		return nil, fmt.Errorf("source not found: %s", sourcePath)
	}

	return sourceInfo.ChunkIDs, nil
}
