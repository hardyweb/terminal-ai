package rag

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type WebScraperConfig struct {
	UserAgent      string
	Timeout        time.Duration
	MaxDepth       int
	AllowedDomains []string
	RateLimit      time.Duration
	ProxyURL       string
}

type WebScrapedContent struct {
	URL         string            `json:"url"`
	Title       string            `json:"title"`
	Content     string            `json:"content"`
	CleanedText string            `json:"cleaned_text"`
	Links       []string          `json:"links"`
	Metadata    map[string]string `json:"metadata"`
	StatusCode  int               `json:"status_code"`
	IndexedAt   time.Time         `json:"indexed_at"`
	ContentHash string            `json:"content_hash"`
}

type WebScraper struct {
	config   WebScraperConfig
	cacheDir string
}

func NewWebScraper() (*WebScraper, error) {
	return NewWebScraperWithConfig(WebScraperConfig{})
}

func NewWebScraperWithConfig(config WebScraperConfig) (*WebScraper, error) {
	if config.UserAgent == "" {
		config.UserAgent = "TerminalAI/1.0 (+https://github.com/anomalyco/terminal-ai)"
	}

	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}

	if config.MaxDepth == 0 {
		config.MaxDepth = 1
	}

	if config.RateLimit == 0 {
		config.RateLimit = time.Second
	}

	cacheDir := filepath.Join(os.Getenv("HOME"), ".local", "share", "terminal-ai", "rag", "web-cache")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, err
	}

	return &WebScraper{
		config:   config,
		cacheDir: cacheDir,
	}, nil
}

func (s *WebScraper) ScrapeURL(ctx context.Context, targetURL string) (*WebScrapedContent, error) {
	if !strings.HasPrefix(targetURL, "http://") && !strings.HasPrefix(targetURL, "https://") {
		targetURL = "https://" + targetURL
	}

	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	client := &http.Client{
		Timeout: s.config.Timeout,
	}

	req, err := http.NewRequestWithContext(ctx, "GET", targetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", s.config.UserAgent)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch URL: %w", err)
	}
	defer resp.Body.Close()

	statusCode := resp.StatusCode
	if statusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", statusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	htmlContent := string(bodyBytes)

	title := extractTitle(htmlContent)
	cleanedText := s.CleanHTML(htmlContent)
	links := extractLinks(htmlContent, targetURL)
	metadata := extractMetadata(htmlContent)

	scrapedContent := &WebScrapedContent{
		URL:         targetURL,
		Title:       title,
		Content:     htmlContent,
		CleanedText: cleanedText,
		Links:       links,
		Metadata:    metadata,
		StatusCode:  statusCode,
		IndexedAt:   time.Now(),
	}

	if title == "" {
		scrapedContent.Title = parsedURL.Hostname()
	}

	return scrapedContent, nil
}

func (s *WebScraper) ScrapeAndIndex(ctx context.Context, targetURL string, ragManager *RAGManager) ([]Chunk, error) {
	scrapedContent, err := s.ScrapeURL(ctx, targetURL)
	if err != nil {
		return nil, err
	}

	chunks, err := s.ChunkWebContent(scrapedContent)
	if err != nil {
		return nil, err
	}

	for i := range chunks {
		if err := SaveChunk(ragManager.GetDataDir(), chunks[i]); err != nil {
			continue
		}
	}

	return chunks, nil
}

func (s *WebScraper) ChunkWebContent(content *WebScrapedContent) ([]Chunk, error) {
	chunker := NewChunker()

	baseName := sanitizeFilename(content.URL)
	sourcePath := fmt.Sprintf("web:%s", baseName)

	chunks, err := chunker.ChunkWithOverlap(content.CleanedText, sourcePath)
	if err != nil {
		return nil, err
	}

	for i := range chunks {
		chunks[i].SourceType = "web"
		chunks[i].SourceURL = content.URL
		chunks[i].Metadata["web_title"] = content.Title
		if content.Metadata["description"] != "" {
			chunks[i].Metadata["web_description"] = content.Metadata["description"]
		}
	}

	return chunks, nil
}

func (s *WebScraper) CleanHTML(htmlContent string) string {
	htmlContent = removeHTMLComments(htmlContent)

	scriptPattern := regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	htmlContent = scriptPattern.ReplaceAllString(htmlContent, "")

	stylePattern := regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	htmlContent = stylePattern.ReplaceAllString(htmlContent, "")

	navPattern := regexp.MustCompile(`(?is)<nav[^>]*>.*?</nav>`)
	htmlContent = navPattern.ReplaceAllString(htmlContent, "\n")

	footerPattern := regexp.MustCompile(`(?is)<footer[^>]*>.*?</footer>`)
	htmlContent = footerPattern.ReplaceAllString(htmlContent, "\n")

	headerPattern := regexp.MustCompile(`(?is)<header[^>]*>.*?</header>`)
	htmlContent = headerPattern.ReplaceAllString(htmlContent, "\n")

	asidePattern := regexp.MustCompile(`(?is)<aside[^>]*>.*?</aside>`)
	htmlContent = asidePattern.ReplaceAllString(htmlContent, "\n")

	adPattern := regexp.MustCompile(`(?is)<div[^>]*class=\"[^\"]*ad[^\"]*\"[^>]*>.*?</div>`)
	htmlContent = adPattern.ReplaceAllString(htmlContent, "\n")

	adPattern2 := regexp.MustCompile(`(?is)<div[^>]*class=\"[^\"]*advertisement[^\"]*\"[^>]*>.*?</div>`)
	htmlContent = adPattern2.ReplaceAllString(htmlContent, "\n")

	htmlContent = strings.ReplaceAll(htmlContent, "\n\n\n", "\n\n")

	text := extractTextFromHTML(htmlContent)

	text = removeExtraWhitespace(text)

	text = strings.TrimSpace(text)

	return text
}

func removeHTMLComments(html string) string {
	commentPattern := regexp.MustCompile(`<!--.*?-->`)
	return commentPattern.ReplaceAllString(html, "")
}

func extractTextFromHTML(html string) string {
	html = strings.ReplaceAll(html, "\r\n", "\n")
	html = strings.ReplaceAll(html, "\r", "\n")

	text := ""
	inTag := false

	for _, char := range html {
		if char == '<' {
			inTag = true
			if text != "" && !strings.HasSuffix(text, "\n") {
				text += "\n"
			}
		} else if char == '>' {
			inTag = false
		} else if !inTag {
			text += string(char)
		}
	}

	return text
}

func removeExtraWhitespace(text string) string {
	spacePattern := regexp.MustCompile(`[ \t]+`)
	text = spacePattern.ReplaceAllString(text, " ")

	newlinePattern := regexp.MustCompile(`\n\s*\n\s*\n`)
	text = newlinePattern.ReplaceAllString(text, "\n\n")

	return text
}

func extractTitle(html string) string {
	titlePattern := regexp.MustCompile(`(?is)<title[^>]*>(.+?)</title>`)
	matches := titlePattern.FindStringSubmatch(html)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}

func extractLinks(html, baseURL string) []string {
	linkPattern := regexp.MustCompile(`(?i)<a[^>]+href=["']([^"']+)["'][^>]*>`)
	matches := linkPattern.FindAllStringSubmatch(html, -1)

	links := make(map[string]bool)
	for _, match := range matches {
		if len(match) > 1 {
			link := strings.TrimSpace(match[1])
			if link != "" && !strings.HasPrefix(link, "javascript:") && !strings.HasPrefix(link, "mailto:") {
				if strings.HasPrefix(link, "http://") || strings.HasPrefix(link, "https://") {
					links[link] = true
				} else if !strings.HasPrefix(link, "#") {
					fullURL, err := url.JoinPath(baseURL, link)
					if err == nil {
						links[fullURL] = true
					}
				}
			}
		}
	}

	result := make([]string, 0, len(links))
	for link := range links {
		result = append(result, link)
	}

	return result
}

func extractMetadata(html string) map[string]string {
	metadata := make(map[string]string)

	descPattern := regexp.MustCompile(`(?i)<meta[^>]+name=["']description["'][^>]+content=["']([^"']+)["'][^>]*>`)
	matches := descPattern.FindStringSubmatch(html)
	if len(matches) > 1 {
		metadata["description"] = strings.TrimSpace(matches[1])
	}

	keywordsPattern := regexp.MustCompile(`(?i)<meta[^>]+name=["']keywords["'][^>]+content=["']([^"']+)["'][^>]*>`)
	matches = keywordsPattern.FindStringSubmatch(html)
	if len(matches) > 1 {
		metadata["keywords"] = strings.TrimSpace(matches[1])
	}

	return metadata
}

func sanitizeFilename(urlStr string) string {
	u, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Sprintf("%x", []byte(urlStr))[:32]
	}

	filename := u.Hostname()
	if filename == "" {
		filename = fmt.Sprintf("%x", []byte(urlStr))[:16]
	}

	filename = strings.ReplaceAll(filename, ".", "_")
	filename = strings.ReplaceAll(filename, ":", "_")

	allowedPattern := regexp.MustCompile(`[^a-zA-Z0-9_-]`)
	filename = allowedPattern.ReplaceAllString(filename, "")

	if len(filename) > 50 {
		filename = filename[:50]
	}

	return filename
}

func (s *WebScraper) CacheWebContent(content WebScrapedContent) error {
	hash := CalculateWebContentHash(content.Content)

	cacheFile := filepath.Join(s.cacheDir, fmt.Sprintf("%s.json", hash[:16]))

	data, err := json.MarshalIndent(content, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(cacheFile, data, 0644)
}

func CalculateWebContentHash(content string) string {
	hash := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", hash)
}

func (s *WebScraper) GetCachedContent(contentHash string) (*WebScrapedContent, error) {
	cacheFile := filepath.Join(s.cacheDir, fmt.Sprintf("%s.json", contentHash[:16]))

	data, err := os.ReadFile(cacheFile)
	if err != nil {
		return nil, err
	}

	var content WebScrapedContent
	if err := json.Unmarshal(data, &content); err != nil {
		return nil, err
	}

	return &content, nil
}

func (s *WebScraper) IsContentCached(contentHash string) bool {
	cacheFile := filepath.Join(s.cacheDir, fmt.Sprintf("%s.json", contentHash[:16]))
	_, err := os.Stat(cacheFile)
	return err == nil
}

func (s *WebScraper) ListCachedContent() ([]WebScrapedContent, error) {
	entries, err := os.ReadDir(s.cacheDir)
	if err != nil {
		return nil, err
	}

	var contents []WebScrapedContent
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		content, err := s.GetCachedContent(strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			continue
		}
		contents = append(contents, *content)
	}

	return contents, nil
}

func (s *WebScraper) ClearCache() error {
	return os.RemoveAll(s.cacheDir)
}

func FetchWebContentSimple(urlStr string) (string, error) {
	if !strings.HasPrefix(urlStr, "http://") && !strings.HasPrefix(urlStr, "https://") {
		urlStr = "https://" + urlStr
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("User-Agent", "TerminalAI/1.0 (+https://github.com/anomalyco/terminal-ai)")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	scraper := &WebScraper{}
	return scraper.CleanHTML(string(body)), nil
}
