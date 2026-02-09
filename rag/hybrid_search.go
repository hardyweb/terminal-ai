package rag

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	RRF_K = float64(60)
)

type HybridSearchConfig struct {
	VectorWeight  float64
	KeywordWeight float64
	TopK          int
	RRF_K         float64
}

type HybridSearchResult struct {
	Chunk        Chunk
	Content      string
	SourcePath   string
	SourceURL    string
	SourceType   string
	VectorScore  float64
	KeywordScore float64
	HybridScore  float64
	Rank         int
	ChunkIndex   int
	TotalChunks  int
}

type KeywordResult struct {
	ID           string
	Content      string
	SourcePath   string
	SourceURL    string
	SourceType   string
	KeywordScore float64
	ChunkIndex   int
	TotalChunks  int
}

type HybridSearchEngine struct {
	ragManager *RAGManager
	config     HybridSearchConfig
}

func NewHybridSearchEngine(ragManager *RAGManager) *HybridSearchEngine {
	return &HybridSearchEngine{
		ragManager: ragManager,
		config: HybridSearchConfig{
			VectorWeight:  0.6,
			KeywordWeight: 0.4,
			TopK:          20,
			RRF_K:         RRF_K,
		},
	}
}

func NewHybridSearchEngineWithConfig(ragManager *RAGManager, config HybridSearchConfig) *HybridSearchEngine {
	if config.VectorWeight == 0 {
		config.VectorWeight = 0.6
	}
	if config.KeywordWeight == 0 {
		config.KeywordWeight = 0.4
	}
	if config.TopK == 0 {
		config.TopK = 20
	}
	if config.RRF_K == 0 {
		config.RRF_K = RRF_K
	}

	return &HybridSearchEngine{
		ragManager: ragManager,
		config:     config,
	}
}

func (e *HybridSearchEngine) Search(ctx context.Context, query string) ([]HybridSearchResult, error) {
	vectorResults, err := e.vectorSearch(ctx, query)
	if err != nil {
		return nil, err
	}

	keywordResults := e.keywordSearch(query)

	mergedResults := e.fuseResults(vectorResults, keywordResults)

	sort.Slice(mergedResults, func(i, j int) bool {
		return mergedResults[i].HybridScore > mergedResults[j].HybridScore
	})

	if len(mergedResults) > e.config.TopK {
		mergedResults = mergedResults[:e.config.TopK]
	}

	for i := range mergedResults {
		mergedResults[i].Rank = i + 1
	}

	return mergedResults, nil
}

func (e *HybridSearchEngine) SearchWithFilters(ctx context.Context, query string, sourceType string) ([]HybridSearchResult, error) {
	allResults, err := e.Search(ctx, query)
	if err != nil {
		return nil, err
	}

	if sourceType == "" {
		return allResults, nil
	}

	var filtered []HybridSearchResult
	for _, result := range allResults {
		if result.SourceType == sourceType {
			filtered = append(filtered, result)
		}
	}

	return filtered, nil
}

func (e *HybridSearchEngine) vectorSearch(ctx context.Context, query string) ([]SearchResult, error) {
	topK := e.config.TopK
	if topK > 50 {
		topK = 50
	}

	return e.ragManager.Search(ctx, query, topK)
}

func (e *HybridSearchEngine) keywordSearch(query string) []KeywordResult {
	queryWords := tokenizeQuery(query)

	chunks, err := LoadAllChunks(e.ragManager.GetDataDir())
	if err != nil {
		return []KeywordResult{}
	}

	type scoredResult struct {
		result KeywordResult
		score  int
	}

	var scoredResults []scoredResult

	for _, chunk := range chunks {
		score := e.calculateKeywordScore(chunk.Content, queryWords)

		if score > 0 {
			scoredResults = append(scoredResults, scoredResult{
				result: KeywordResult{
					ID:           chunk.ID,
					Content:      chunk.Content,
					SourcePath:   chunk.SourcePath,
					SourceURL:    chunk.SourceURL,
					SourceType:   chunk.SourceType,
					KeywordScore: 0,
					ChunkIndex:   chunk.ChunkIndex,
					TotalChunks:  chunk.TotalChunks,
				},
				score: score,
			})
		}
	}

	sort.Slice(scoredResults, func(i, j int) bool {
		return scoredResults[i].score > scoredResults[j].score
	})

	maxScore := 0
	for _, sr := range scoredResults {
		if sr.score > maxScore {
			maxScore = sr.score
		}
	}

	var results []KeywordResult
	for _, sr := range scoredResults {
		var keywordScore float64
		if maxScore > 0 {
			keywordScore = float64(sr.score) / float64(maxScore)
		}
		sr.result.KeywordScore = keywordScore
		results = append(results, sr.result)
	}

	return results
}

func (e *HybridSearchEngine) calculateKeywordScore(content string, queryWords []string) int {
	contentLower := strings.ToLower(content)

	score := 0
	wordCounts := make(map[string]int)

	for _, word := range queryWords {
		count := strings.Count(contentLower, word)
		wordCounts[word] = count
	}

	for _, count := range wordCounts {
		if count > 0 {
			score += count
		}
	}

	return score
}

func (e *HybridSearchEngine) fuseResults(vectorResults []SearchResult, keywordResults []KeywordResult) []HybridSearchResult {
	keywordMap := make(map[string]KeywordResult)
	for _, kr := range keywordResults {
		keywordMap[kr.ID] = kr
	}

	seenIDs := make(map[string]bool)
	var results []HybridSearchResult

	vectorRank := 0
	for _, vr := range vectorResults {
		vectorRank++

		kr, hasKeyword := keywordMap[vr.Chunk.ID]
		if hasKeyword {
			delete(keywordMap, vr.Chunk.ID)
		}

		keywordScore := float64(0)
		if hasKeyword {
			keywordScore = kr.KeywordScore
		}

		hybridScore := e.calculateHybridScore(float64(vr.Similarity), keywordScore)

		results = append(results, HybridSearchResult{
			Chunk:        vr.Chunk,
			Content:      vr.Content,
			SourcePath:   vr.SourcePath,
			SourceURL:    vr.SourceURL,
			SourceType:   vr.SourceType,
			VectorScore:  vr.Similarity,
			KeywordScore: keywordScore,
			HybridScore:  hybridScore,
		})

		seenIDs[vr.Chunk.ID] = true
	}

	remainingKeywordRank := len(vectorResults) + 1
	for _, kr := range keywordResults {
		if seenIDs[kr.ID] {
			continue
		}

		hybridScore := e.calculateHybridScore(0, kr.KeywordScore)

		results = append(results, HybridSearchResult{
			Chunk: Chunk{
				ID:         kr.ID,
				Content:    kr.Content,
				SourcePath: kr.SourcePath,
				SourceURL:  kr.SourceURL,
				SourceType: kr.SourceType,
			},
			Content:      kr.Content,
			SourcePath:   kr.SourcePath,
			SourceURL:    kr.SourceURL,
			SourceType:   kr.SourceType,
			VectorScore:  0,
			KeywordScore: kr.KeywordScore,
			HybridScore:  hybridScore,
			ChunkIndex:   kr.ChunkIndex,
			TotalChunks:  kr.TotalChunks,
		})

		remainingKeywordRank++
	}

	return results
}

func (e *HybridSearchEngine) calculateHybridScore(vectorScore, keywordScore float64) float64 {
	vw := e.config.VectorWeight
	kw := e.config.KeywordWeight

	vectorRRF := 0.0
	if vectorScore > 0 {
		vectorRRF = 1.0 / (e.config.RRF_K + (1.0 - vectorScore))
	}

	keywordRRF := 0.0
	if keywordScore > 0 {
		keywordRRF = 1.0 / (e.config.RRF_K + (1.0 - keywordScore))
	}

	return (vectorRRF * vw) + (keywordRRF * kw)
}

func tokenizeQuery(query string) []string {
	query = strings.ToLower(query)

	wordPattern := regexp.MustCompile(`\b\w+\b`)
	words := wordPattern.FindAllString(query, -1)

	stopWords := map[string]bool{
		"the":    true,
		"a":      true,
		"an":     true,
		"and":    true,
		"or":     true,
		"but":    true,
		"in":     true,
		"on":     true,
		"at":     true,
		"to":     true,
		"for":    true,
		"of":     true,
		"with":   true,
		"by":     true,
		"from":   true,
		"is":     true,
		"are":    true,
		"was":    true,
		"were":   true,
		"be":     true,
		"been":   true,
		"being":  true,
		"have":   true,
		"has":    true,
		"had":    true,
		"do":     true,
		"does":   true,
		"did":    true,
		"will":   true,
		"would":  true,
		"could":  true,
		"should": true,
		"may":    true,
		"might":  true,
		"must":   true,
		"shall":  true,
		"can":    true,
		"this":   true,
		"that":   true,
		"these":  true,
		"those":  true,
		"i":      true,
		"you":    true,
		"he":     true,
		"she":    true,
		"it":     true,
		"we":     true,
		"they":   true,
		"what":   true,
		"which":  true,
		"who":    true,
		"whom":   true,
		"whose":  true,
		"where":  true,
		"when":   true,
		"why":    true,
		"how":    true,
		"all":    true,
		"each":   true,
		"every":  true,
		"both":   true,
		"few":    true,
		"more":   true,
		"most":   true,
		"other":  true,
		"some":   true,
		"such":   true,
		"no":     true,
		"nor":    true,
		"not":    true,
		"only":   true,
		"own":    true,
		"same":   true,
		"so":     true,
		"than":   true,
		"too":    true,
		"very":   true,
		"just":   true,
	}

	var filtered []string
	for _, word := range words {
		if !stopWords[word] && len(word) > 1 {
			filtered = append(filtered, word)
		}
	}

	if len(filtered) == 0 {
		return words
	}

	return filtered
}

func (e *HybridSearchEngine) GetStats() HybridSearchStats {
	return HybridSearchStats{
		VectorWeight:  e.config.VectorWeight,
		KeywordWeight: e.config.KeywordWeight,
		TopK:          e.config.TopK,
		RRF_K:         e.config.RRF_K,
	}
}

type HybridSearchStats struct {
	VectorWeight  float64 `json:"vector_weight"`
	KeywordWeight float64 `json:"keyword_weight"`
	TopK          int     `json:"top_k"`
	RRF_K         float64 `json:"rrf_k"`
}

type SearchStats struct {
	TotalResults int           `json:"total_results"`
	SearchTime   time.Duration `json:"search_time"`
	QueryTime    time.Time     `json:"query_time"`
}

func (e *HybridSearchEngine) SearchWithStats(ctx context.Context, query string) ([]HybridSearchResult, SearchStats, error) {
	startTime := time.Now()

	results, err := e.Search(ctx, query)
	if err != nil {
		return nil, SearchStats{}, err
	}

	stats := SearchStats{
		TotalResults: len(results),
		SearchTime:   time.Since(startTime),
		QueryTime:    time.Now(),
	}

	return results, stats, nil
}

func FormatSearchResult(result HybridSearchResult, showScore bool) string {
	sourceType := "📄"
	if result.SourceType == "web" {
		sourceType = "🌐"
	}

	sourceName := result.SourcePath
	if result.SourceType == "web" && result.SourceURL != "" {
		sourceName = result.SourceURL
	}

	var output string
	if len(sourceName) > 50 {
		sourceName = sourceName[:47] + "..."
	}

	output = fmt.Sprintf("%s %s", sourceType, sourceName)

	if showScore {
		output = fmt.Sprintf("%s [Score: %.3f]", output, result.HybridScore)
	}

	content := result.Content
	if len(content) > 150 {
		content = content[:147] + "..."
	}

	output = fmt.Sprintf("%s\n   %s", output, content)

	return output
}

func FormatSearchResults(results []HybridSearchResult, query string, duration time.Duration) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("🔍 Search: %s\n", query))
	sb.WriteString("═══════════════════════════════════════════\n\n")

	if len(results) == 0 {
		sb.WriteString("No results found.\n")
		return sb.String()
	}

	for i, result := range results {
		if i > 0 {
			sb.WriteString("\n")
		}

		sourceType := "📄"
		if result.SourceType == "web" {
			sourceType = "🌐"
		}

		sourceName := result.SourcePath
		if result.SourceType == "web" && result.SourceURL != "" {
			sourceName = result.SourceURL
		}

		if len(sourceName) > 60 {
			sourceName = sourceName[:57] + "..."
		}

		sb.WriteString(fmt.Sprintf("%d. %s %s\n", i+1, sourceType, sourceName))

		sb.WriteString(fmt.Sprintf("   Score: %.3f (Vector: %.2f | Keyword: %.2f)\n",
			result.HybridScore, result.VectorScore, result.KeywordScore))

		content := result.Content
		if len(content) > 200 {
			content = content[:197] + "..."
		}
		sb.WriteString(fmt.Sprintf("   %s\n", content))
	}

	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("Showing top %d of %d results (%.2fs)\n", len(results), len(results), duration.Seconds()))

	return sb.String()
}
