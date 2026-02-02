package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type AIProvider struct {
	Name     string
	APIKey   string
	Endpoint string
	Model    string
}

type AIProviderConfig struct {
	Priority    int                   `json:"priority"`
	Enabled     bool                  `json:"enabled"`
	MaxRetries  int                   `json:"max_retries"`
	GopassKey   string                `json:"gopass_key"`
	EnvKey      string                `json:"env_key"`
	EndpointKey string                `json:"endpoint_key"`
	ModelKey    string                `json:"model_key"`
	BYOK        bool                  `json:"byok"`
	Description string                `json:"description"`
	BYOKConfig  *OpenRouterBYOKConfig `json:"byok_config,omitempty"`
}

type OpenRouterBYOKConfig struct {
	Enabled               bool              `json:"enabled"`
	ProviderOrder         []string          `json:"provider_order"`
	AllowFallbackToShared bool              `json:"allow_fallback_to_shared"`
	Models                map[string]string `json:"models"`
}

type ProviderGlobalConfig struct {
	DefaultProvider string                      `json:"default_provider"`
	FallbackEnabled bool                        `json:"fallback_enabled"`
	RetryAttempts   int                         `json:"retry_attempts"`
	RetryDelayMs    int                         `json:"retry_delay_ms"`
	Providers       map[string]AIProviderConfig `json:"providers"`
}

type ProviderError struct {
	Provider string
	Error    error
	Type     string
}

type Request struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream,omitempty"`
}

type OpenRouterProvider struct {
	AllowFallbacks bool     `json:"allow_fallbacks"`
	Order          []string `json:"order"`
}

type OpenRouterRequest struct {
	Model    string              `json:"model"`
	Messages []Message           `json:"messages"`
	Stream   bool                `json:"stream,omitempty"`
	Provider *OpenRouterProvider `json:"provider,omitempty"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Response struct {
	Choices []Choice  `json:"choices"`
	Error   *APIError `json:"error,omitempty"`
}

type Choice struct {
	Message Message `json:"message"`
}

type StreamingDelta struct {
	Content string `json:"content"`
}

type StreamingChoice struct {
	Delta StreamingDelta `json:"delta"`
}

type StreamingResponse struct {
	Choices []StreamingChoice `json:"choices"`
	Error   *APIError         `json:"error,omitempty"`
}

type APIError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

type RAGDocument struct {
	Path       string   `json:"path"`
	Content    string   `json:"content"`
	Keywords   []string `json:"keywords"`
	IndexedAt  string   `json:"indexed_at"`
	Owner      string   `json:"owner"`
	Visibility string   `json:"visibility"`
}

type RAGIndex struct {
	Documents []RAGDocument `json:"documents"`
}

type Skill struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Triggers    []string `json:"triggers"`
	Template    string   `json:"template"`
}

type ChatMessage struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	Timestamp string `json:"timestamp"`
}

type ChatSession struct {
	ID        string        `json:"id"`
	Title     string        `json:"title"`
	Provider  string        `json:"provider"`
	User      string        `json:"user"`
	CreatedAt string        `json:"created_at"`
	UpdatedAt string        `json:"updated_at"`
	Messages  []ChatMessage `json:"messages"`
}

type ChatHistory struct {
	Sessions []ChatSession `json:"sessions"`
}

var ragIndex RAGIndex
var chatHistory ChatHistory
var providers map[string]AIProvider
var useGopass bool
var providerConfig ProviderGlobalConfig
var streamingEnabled bool

func getSecretFromGopass(path string) (string, error) {
	cmd := exec.Command("gopass", "show", path)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("gopass show failed: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

func getEnvOrGopass(envVar, gopassPath string) string {
	val := os.Getenv(envVar)
	if val != "" && !strings.HasPrefix(val, "gopass:") {
		return val
	}

	if strings.HasPrefix(val, "gopass:") {
		path := strings.TrimPrefix(val, "gopass:")
		if secret, err := getSecretFromGopass(path); err == nil {
			return secret
		}
	}

	if useGopass && gopassPath != "" {
		if secret, err := getSecretFromGopass(gopassPath); err == nil {
			return secret
		}
	}

	return ""
}

func loadProviderConfig() error {
	homeDir, _ := os.UserHomeDir()
	configFile := filepath.Join(homeDir, configDir, "providers.json")

	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		return createDefaultProviderConfig(configFile)
	}

	data, err := os.ReadFile(configFile)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, &providerConfig)
}

func createDefaultProviderConfig(path string) error {
	homeDir, _ := os.UserHomeDir()
	configPath := filepath.Join(homeDir, configDir)
	os.MkdirAll(configPath, 0755)

	defaultConfig := ProviderGlobalConfig{
		DefaultProvider: "openrouter",
		FallbackEnabled: true,
		RetryAttempts:   3,
		RetryDelayMs:    1000,
		Providers: map[string]AIProviderConfig{
			"openrouter": {
				Priority:    1,
				Enabled:     true,
				MaxRetries:  2,
				GopassKey:   "terminal-ai/openrouter_api_key",
				EnvKey:      "OPENROUTER_API_KEY",
				EndpointKey: "OPENROUTER_ENDPOINT",
				ModelKey:    "OPENROUTER_MODEL",
				// BYOK Configuration (disabled by default)
				// To enable BYOK, uncomment and configure:
				// BYOKConfig: &OpenRouterBYOKConfig{
				// 	Enabled:               true,
				// 	ProviderOrder:         []string{"Cerebras", "Google AI Studio", "Groq"},
				// 	AllowFallbackToShared: true,
				// 	Models: map[string]string{
				// 		"cerebras": "cerebras/llama-3.1-8b",
				// 		"google":   "google/gemini-2.0-flash-exp:free",
				// 		"groq":     "groq/llama-3.3-70b-versatile",
				// 	},
				// },
			},
			"gemini": {
				Priority:    2,
				Enabled:     true,
				MaxRetries:  2,
				GopassKey:   "terminal-ai/gemini_api_key",
				EnvKey:      "GEMINI_API_KEY",
				EndpointKey: "GEMINI_ENDPOINT",
				ModelKey:    "GEMINI_MODEL",
			},
			"groq": {
				Priority:    3,
				Enabled:     true,
				MaxRetries:  2,
				GopassKey:   "terminal-ai/groq_api_key",
				EnvKey:      "GROQ_API_KEY",
				EndpointKey: "GROQ_ENDPOINT",
				ModelKey:    "GROQ_MODEL",
			},
		},
	}

	data, _ := json.MarshalIndent(defaultConfig, "", "  ")
	return os.WriteFile(path, data, 0644)
}

func getOrderedProviders() []string {
	type providerPriority struct {
		name     string
		priority int
	}

	var priorities []providerPriority
	for name, config := range providerConfig.Providers {
		if config.Enabled {
			priorities = append(priorities, providerPriority{name, config.Priority})
		}
	}

	sort.Slice(priorities, func(i, j int) bool {
		return priorities[i].priority < priorities[j].priority
	})

	var result []string
	for _, p := range priorities {
		result = append(result, p.name)
	}
	return result
}

func classifyError(err error, response *Response) string {
	if err != nil {
		if strings.Contains(err.Error(), "timeout") || strings.Contains(err.Error(), "deadline exceeded") {
			return "timeout"
		}
		if strings.Contains(err.Error(), "connection") || strings.Contains(err.Error(), "network") {
			return "network"
		}
	}
	if response != nil && response.Error != nil {
		if strings.Contains(response.Error.Type, "rate_limit") ||
			strings.Contains(response.Error.Message, "rate limit") ||
			strings.Contains(response.Error.Message, "429") {
			return "rate_limit"
		}
		return "server_error"
	}
	return "unknown"
}

func combineErrors(err error, response *Response) error {
	if err != nil && response != nil && response.Error != nil {
		return fmt.Errorf("%v: %s", err, response.Error.Message)
	}
	if err != nil {
		return err
	}
	if response != nil && response.Error != nil {
		return fmt.Errorf(response.Error.Message)
	}
	return fmt.Errorf("unknown error")
}

func main() {
	homeDir, _ := os.UserHomeDir()
	configPath := filepath.Join(homeDir, configDir)

	godotenv.Load(filepath.Join(configPath, ".env"))
	godotenv.Load(".env")

	useGopass = os.Getenv("USE_GOPASS") == "true"
	streamingEnabled = os.Getenv("STREAMING") != "false" // Default to true if not set or set to true

	if streamingEnabled {
		fmt.Println("✅ Streaming mode enabled (chunk by chunk response)")
	}

	if err := loadProviderConfig(); err != nil {
		fmt.Printf("Warning: Failed to load provider config: %v\n", err)
	}

	initProviders()
	securityMgr = initSecurityManager()
	loadRAGIndex()
	loadChatHistory()

	if len(os.Args) < 2 {
		showHelp()
		os.Exit(0)
	}

	cmd := os.Args[1]

	switch cmd {
	case "web":
		if len(os.Args) < 3 {
			fmt.Println("Usage: terminal-ai web <url>")
			os.Exit(1)
		}
		fetchWebContent(os.Args[2])
	case "rag":
		handleRAGCommand()
	case "skill":
		handleSkillCommand()
	case "user":
		handleUserCommand()
	case "provider":
		handleProviderCommand()
	case "web-server":
		startWebServer()
	case "chat":
		handleChatCommand()
	case "history":
		handleHistoryCommand()
	case "--help", "-h":
		showHelp()
	default:
		if cmd == "openrouter" || cmd == "gemini" || cmd == "groq" {
			provider := cmd
			message := strings.Join(os.Args[2:], " ")
			chatWithAI(provider, message)
		} else {
			chatWithAI("openrouter", strings.Join(os.Args[1:], " "))
		}
	}
}

func initProviders() {
	providers = map[string]AIProvider{
		"openrouter": {
			Name:     "openrouter",
			APIKey:   getEnvOrGopass("OPENROUTER_API_KEY", "terminal-ai/openrouter_api_key"),
			Endpoint: os.Getenv("OPENROUTER_ENDPOINT"),
			Model:    os.Getenv("OPENROUTER_MODEL"),
		},
		"gemini": {
			Name:     "gemini",
			APIKey:   getEnvOrGopass("GEMINI_API_KEY", "terminal-ai/gemini_api_key"),
			Endpoint: os.Getenv("GEMINI_ENDPOINT"),
			Model:    os.Getenv("GEMINI_MODEL"),
		},
		"groq": {
			Name:     "groq",
			APIKey:   getEnvOrGopass("GROQ_API_KEY", "terminal-ai/groq_api_key"),
			Endpoint: os.Getenv("GROQ_ENDPOINT"),
			Model:    os.Getenv("GROQ_MODEL"),
		},
	}
}

func getDataDir() string {
	if xdgData := os.Getenv("XDG_DATA_HOME"); xdgData != "" {
		return filepath.Join(xdgData, "terminal-ai")
	}
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".local", "share", "terminal-ai")
}

func loadRAGIndex() {
	dataDir := getDataDir()
	indexFile := filepath.Join(dataDir, "rag-index.json")

	data, err := os.ReadFile(indexFile)
	if err != nil {
		ragIndex = RAGIndex{Documents: []RAGDocument{}}
		return
	}

	json.Unmarshal(data, &ragIndex)
}

func saveRAGIndex() error {
	dataDir := getDataDir()
	indexFile := filepath.Join(dataDir, "rag-index.json")

	os.MkdirAll(dataDir, 0755)

	data, err := json.MarshalIndent(ragIndex, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(indexFile, data, 0644)
}

func getChatHistoryPath() string {
	dataDir := getDataDir()
	return filepath.Join(dataDir, "chat-history.json")
}

func loadChatHistory() error {
	data, err := os.ReadFile(getChatHistoryPath())
	if err != nil {
		chatHistory = ChatHistory{Sessions: []ChatSession{}}
		return nil
	}
	return json.Unmarshal(data, &chatHistory)
}

func saveChatHistory() error {
	dataDir := getDataDir()
	historyFile := getChatHistoryPath()

	os.MkdirAll(dataDir, 0755)

	data, err := json.MarshalIndent(chatHistory, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(historyFile, data, 0644)
}

func generateSessionID() string {
	return fmt.Sprintf("chat_%d", time.Now().UnixNano())
}

func createSession(title, provider, user string) *ChatSession {
	session := ChatSession{
		ID:        generateSessionID(),
		Title:     title,
		Provider:  provider,
		User:      user,
		CreatedAt: time.Now().Format(time.RFC3339),
		UpdatedAt: time.Now().Format(time.RFC3339),
		Messages:  []ChatMessage{},
	}

	chatHistory.Sessions = append(chatHistory.Sessions, session)
	saveChatHistory()
	return &session
}

func updateSession(sessionID, role, content string) error {
	for i := range chatHistory.Sessions {
		if chatHistory.Sessions[i].ID == sessionID {
			message := ChatMessage{
				Role:      role,
				Content:   content,
				Timestamp: time.Now().Format(time.RFC3339),
			}
			chatHistory.Sessions[i].Messages = append(chatHistory.Sessions[i].Messages, message)
			chatHistory.Sessions[i].UpdatedAt = time.Now().Format(time.RFC3339)
			return saveChatHistory()
		}
	}
	return fmt.Errorf("session not found")
}

func getSession(sessionID string) (*ChatSession, error) {
	for i := range chatHistory.Sessions {
		if chatHistory.Sessions[i].ID == sessionID {
			return &chatHistory.Sessions[i], nil
		}
	}
	return nil, fmt.Errorf("session not found")
}

func listSessions() []ChatSession {
	sort.Slice(chatHistory.Sessions, func(i, j int) bool {
		timeI, _ := time.Parse(time.RFC3339, chatHistory.Sessions[i].UpdatedAt)
		timeJ, _ := time.Parse(time.RFC3339, chatHistory.Sessions[j].UpdatedAt)
		return timeJ.Before(timeI)
	})
	return chatHistory.Sessions
}

func deleteSession(sessionID string) error {
	for i, session := range chatHistory.Sessions {
		if session.ID == sessionID {
			chatHistory.Sessions = append(chatHistory.Sessions[:i], chatHistory.Sessions[i+1:]...)
			return saveChatHistory()
		}
	}
	return fmt.Errorf("session not found")
}

func clearAllHistory() error {
	chatHistory = ChatHistory{Sessions: []ChatSession{}}
	return saveChatHistory()
}

func getLatestSession() *ChatSession {
	sessions := listSessions()
	if len(sessions) == 0 {
		return nil
	}
	return &sessions[0]
}

func truncateTitle(title string) string {
	if len(title) <= 100 {
		return title
	}
	return title[:100]
}

func fetchWebContent(url string) {
	resp, err := http.Get(url)
	if err != nil {
		fmt.Printf("Error fetching URL: %v\n", err)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error reading response: %v\n", err)
		return
	}

	fmt.Println(string(body))
}

func handleRAGCommand() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: terminal-ai rag index <dir> | terminal-ai rag search <query>")
		os.Exit(1)
	}

	subCmd := os.Args[2]

	switch subCmd {
	case "index":
		if len(os.Args) < 4 {
			fmt.Println("Usage: terminal-ai rag index <dir>")
			os.Exit(1)
		}
		indexDirectory(os.Args[3])
	case "search":
		if len(os.Args) < 4 {
			fmt.Println("Usage: terminal-ai rag search <query>")
			os.Exit(1)
		}
		results := searchRAG(os.Args[3])
		if len(results) == 0 {
			fmt.Println("No results found")
		} else {
			fmt.Printf("🔍 Found %d result(s):\n\n", len(results))
			for i, doc := range results {
				fmt.Printf("%d. %s\n", i+1, doc.Path)
				contentPreview := doc.Content
				if len(contentPreview) > 100 {
					contentPreview = contentPreview[:100] + "..."
				}
				fmt.Printf("   %s\n\n", contentPreview)
			}
		}
	default:
		fmt.Println("Unknown RAG command. Use: index | search")
	}
}

func indexDirectory(dir string) {
	indexDirectoryWithOwner(dir, "", "private")
}

func indexDirectoryWithOwner(dir, owner, visibility string) {
	count := 0
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".txt" && ext != ".md" && ext != ".json" {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		keywords := extractKeywords(string(content))

		doc := RAGDocument{
			Path:       path,
			Content:    string(content),
			Keywords:   keywords,
			IndexedAt:  time.Now().Format(time.RFC3339),
			Owner:      owner,
			Visibility: visibility,
		}

		ragIndex.Documents = append(ragIndex.Documents, doc)
		count++
		return nil
	})

	if err != nil {
		fmt.Printf("Error indexing directory: %v\n", err)
		return
	}

	if err := saveRAGIndex(); err != nil {
		fmt.Printf("Error saving index: %v\n", err)
		return
	}

	fmt.Printf("✅ Indexed %d documents (owner: %s, visibility: %s)\n", count, owner, visibility)
}

func searchRAG(query string) []RAGDocument {
	return searchRAGWithFilters(query, "", "")
}

func searchRAGWithFilters(query, username, visibility string) []RAGDocument {
	queryWords := tokenize(query)
	type scoreDoc struct {
		doc   RAGDocument
		score int
	}
	var scored []scoreDoc

	for _, doc := range ragIndex.Documents {
		canAccess := false

		if username == "" && visibility == "" {
			canAccess = true
		} else if visibility == "public" {
			canAccess = doc.Visibility == "public"
		} else if username != "" {
			if doc.Visibility == "public" {
				canAccess = true
			} else if doc.Owner == username {
				canAccess = true
			}
		}

		if !canAccess {
			continue
		}

		score := 0
		docKeywords := make(map[string]bool)
		for _, kw := range doc.Keywords {
			docKeywords[strings.ToLower(kw)] = true
		}

		for _, qw := range queryWords {
			if docKeywords[strings.ToLower(qw)] {
				score++
			}
		}

		if score > 0 {
			scored = append(scored, scoreDoc{doc, score})
		}
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	var results []RAGDocument
	maxResults := 3
	if len(scored) < maxResults {
		maxResults = len(scored)
	}

	for i := 0; i < maxResults; i++ {
		results = append(results, scored[i].doc)
	}

	return results
}

func extractKeywords(text string) []string {
	words := tokenize(text)
	freq := make(map[string]int)

	for _, word := range words {
		freq[strings.ToLower(word)]++
	}

	type wordFreq struct {
		word  string
		count int
	}

	var sorted []wordFreq
	for word, count := range freq {
		sorted = append(sorted, wordFreq{word, count})
	}

	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].count > sorted[j].count
	})

	var keywords []string
	maxKeywords := 10
	if len(sorted) < maxKeywords {
		maxKeywords = len(sorted)
	}

	for i := 0; i < maxKeywords; i++ {
		keywords = append(keywords, sorted[i].word)
	}

	return keywords
}

func tokenize(text string) []string {
	re := regexp.MustCompile(`\b\w+\b`)
	return re.FindAllString(text, -1)
}

func handleSkillCommand() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: terminal-ai skill list | skill create <name>")
		os.Exit(1)
	}

	subCmd := os.Args[2]

	switch subCmd {
	case "list":
		listSkills()
	case "create":
		if len(os.Args) < 4 {
			fmt.Println("Usage: terminal-ai skill create <name>")
			os.Exit(1)
		}
		createSkill(os.Args[3])
	default:
		fmt.Println("Unknown skill command. Use: list | create")
	}
}

func listSkills() {
	homeDir, _ := os.UserHomeDir()
	skillsDir := filepath.Join(homeDir, configDir, "skills")

	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		fmt.Println("No skills found")
		return
	}

	fmt.Println("Available Skills:")
	for _, entry := range entries {
		if entry.IsDir() {
			skillFile := filepath.Join(skillsDir, entry.Name(), "skill.json")
			data, err := os.ReadFile(skillFile)
			if err == nil {
				var skill Skill
				json.Unmarshal(data, &skill)
				fmt.Printf("  - %s: %s\n", skill.Name, skill.Description)
			}
		}
	}
}

func createSkill(name string) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Print("Description: ")
	desc, _ := reader.ReadString('\n')
	desc = strings.TrimSpace(desc)

	fmt.Print("Triggers (comma-separated): ")
	triggersStr, _ := reader.ReadString('\n')
	triggersStr = strings.TrimSpace(triggersStr)
	triggers := strings.Split(triggersStr, ",")
	for i := range triggers {
		triggers[i] = strings.TrimSpace(triggers[i])
	}

	fmt.Print("Template: ")
	template, _ := reader.ReadString('\n')
	template = strings.TrimSpace(template)

	skill := Skill{
		Name:        name,
		Description: desc,
		Triggers:    triggers,
		Template:    template,
	}

	data, _ := json.MarshalIndent(skill, "", "  ")

	homeDir, _ := os.UserHomeDir()
	skillDir := filepath.Join(homeDir, configDir, "skills", name)
	os.MkdirAll(skillDir, 0755)

	skillFile := filepath.Join(skillDir, "skill.json")
	os.WriteFile(skillFile, data, 0644)

	fmt.Printf("✅ Skill '%s' created successfully\n", name)
}

func handleUserCommand() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: terminal-ai user list | user create <name> <role> | user delete <name>")
		os.Exit(1)
	}

	subCmd := os.Args[2]

	switch subCmd {
	case "list":
		listUsers()
	case "create":
		if len(os.Args) < 5 {
			fmt.Println("Usage: terminal-ai user create <name> <role>")
			os.Exit(1)
		}
		fmt.Print("Password: ")
		var password string
		fmt.Scanln(&password)
		securityMgr.CreateUser(os.Args[3], password, os.Args[4])
		fmt.Printf("✅ User '%s' created\n", os.Args[3])
	case "delete":
		if len(os.Args) < 4 {
			fmt.Println("Usage: terminal-ai user delete <name>")
			os.Exit(1)
		}
		deleteUser(os.Args[3])
	default:
		fmt.Println("Unknown user command. Use: list | create | delete")
	}
}

func listUsers() {
	for username, user := range securityMgr.users {
		fmt.Printf("  - %s (%s)\n", username, user.Role)
	}
}

func deleteUser(username string) {
	delete(securityMgr.users, username)
	securityMgr.saveUsers()
	fmt.Printf("✅ User '%s' deleted\n", username)
}

func handleProviderCommand() {
	if len(os.Args) < 3 {
		showProviderHelp()
		os.Exit(1)
	}

	subCmd := os.Args[2]

	switch subCmd {
	case "list":
		listProviders()
	case "test":
		if len(os.Args) < 4 {
			fmt.Println("Usage: terminal-ai provider test <provider-name>")
			os.Exit(1)
		}
		testProvider(os.Args[3])
	case "enable":
		if len(os.Args) < 4 {
			fmt.Println("Usage: terminal-ai provider enable <provider-name>")
			os.Exit(1)
		}
		toggleProvider(os.Args[3], true)
	case "disable":
		if len(os.Args) < 4 {
			fmt.Println("Usage: terminal-ai provider disable <provider-name>")
			os.Exit(1)
		}
		toggleProvider(os.Args[3], false)
	case "priority":
		if len(os.Args) < 5 {
			fmt.Println("Usage: terminal-ai provider priority <provider-name> <priority>")
			os.Exit(1)
		}
		var priority int
		fmt.Sscanf(os.Args[4], "%d", &priority)
		setProviderPriority(os.Args[3], priority)
	case "add":
		if len(os.Args) < 4 {
			fmt.Println("Usage: terminal-ai provider add <provider-name>")
			os.Exit(1)
		}
		addProvider(os.Args[3])
	case "default":
		if len(os.Args) < 4 {
			fmt.Println("Usage: terminal-ai provider default <provider-name>")
			os.Exit(1)
		}
		setDefaultProvider(os.Args[3])
	case "byok":
		handleBYOKCommand()
	default:
		showProviderHelp()
	}
}

func listProviders() {
	fmt.Println("📊 Provider Configuration:")
	fmt.Println()

	orderedProviders := getOrderedProviders()

	for i, providerName := range orderedProviders {
		config := providerConfig.Providers[providerName]
		provider := providers[providerName]

		status := "✅ Enabled"
		if !config.Enabled {
			status = "❌ Disabled"
		}

		defaultMarker := ""
		if providerName == providerConfig.DefaultProvider {
			defaultMarker = " (DEFAULT)"
		}

		fmt.Printf("%d. %s%s\n", i+1, providerName, defaultMarker)
		fmt.Printf("   Priority: %d | %s\n", config.Priority, status)
		fmt.Printf("   Endpoint: %s\n", provider.Endpoint)
		fmt.Printf("   Model: %s\n", provider.Model)
		if config.BYOK {
			fmt.Printf("   🔐 BYOK: Custom provider\n")
		}
		fmt.Printf("   Max Retries: %d\n", config.MaxRetries)
		fmt.Println()
	}

	fmt.Printf("Fallback Enabled: %v\n", providerConfig.FallbackEnabled)
	fmt.Printf("Default Provider: %s\n", providerConfig.DefaultProvider)
}

func testProvider(providerName string) {
	config, exists := providerConfig.Providers[providerName]
	if !exists {
		fmt.Printf("❌ Provider '%s' not found\n", providerName)
		return
	}

	if !config.Enabled {
		fmt.Printf("❌ Provider '%s' is disabled\n", providerName)
		return
	}

	provider, exists := providers[providerName]
	if !exists {
		fmt.Printf("❌ Provider '%s' not initialized\n", providerName)
		return
	}

	if provider.APIKey == "" {
		fmt.Printf("❌ No API key configured for %s\n", providerName)
		return
	}

	fmt.Printf("🧪 Testing provider: %s\n", providerName)
	fmt.Printf("   Endpoint: %s\n", provider.Endpoint)
	fmt.Printf("   Model: %s\n", provider.Model)
	fmt.Println()

	req := Request{
		Model: provider.Model,
		Messages: []Message{
			{Role: "user", Content: "Hello! Say 'Test successful' if you receive this."},
		},
		Stream: false,
	}

	response, err := makeRequest(provider.Endpoint, provider.APIKey, req, provider.Name)

	if err != nil {
		fmt.Printf("❌ Test failed: %v\n", err)
		errorType := classifyError(err, response)
		fmt.Printf("   Error type: %s\n", errorType)
		return
	}

	if response.Error != nil {
		fmt.Printf("❌ API Error: %s\n", response.Error.Message)
		return
	}

	if len(response.Choices) > 0 {
		fmt.Printf("✅ Test successful!\n")
		fmt.Printf("   Response: %s\n", response.Choices[0].Message.Content[:min(100, len(response.Choices[0].Message.Content))])
	} else {
		fmt.Printf("❌ No response received\n")
	}
}

func toggleProvider(providerName string, enabled bool) {
	config, exists := providerConfig.Providers[providerName]
	if !exists {
		fmt.Printf("❌ Provider '%s' not found\n", providerName)
		return
	}

	config.Enabled = enabled
	providerConfig.Providers[providerName] = config

	if err := saveProviderConfig(); err != nil {
		fmt.Printf("❌ Failed to update provider: %v\n", err)
		return
	}

	status := "enabled"
	if !enabled {
		status = "disabled"
	}
	fmt.Printf("✅ Provider '%s' %s\n", providerName, status)
}

func setProviderPriority(providerName string, priority int) {
	config, exists := providerConfig.Providers[providerName]
	if !exists {
		fmt.Printf("❌ Provider '%s' not found\n", providerName)
		return
	}

	config.Priority = priority
	providerConfig.Providers[providerName] = config

	if err := saveProviderConfig(); err != nil {
		fmt.Printf("❌ Failed to update priority: %v\n", err)
		return
	}

	fmt.Printf("✅ Provider '%s' priority set to %d\n", providerName, priority)
}

func addProvider(providerName string) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Printf("🔧 Adding new provider: %s\n", providerName)
	fmt.Println()

	fmt.Print("Priority (0=highest): ")
	priorityStr, _ := reader.ReadString('\n')
	priority := 1
	fmt.Sscanf(strings.TrimSpace(priorityStr), "%d", &priority)

	fmt.Print("Endpoint URL: ")
	endpoint, _ := reader.ReadString('\n')
	endpoint = strings.TrimSpace(endpoint)

	fmt.Print("Model name: ")
	model, _ := reader.ReadString('\n')
	model = strings.TrimSpace(model)

	fmt.Print("API Key (or leave blank to use gopass): ")
	apiKey, _ := reader.ReadString('\n')
	apiKey = strings.TrimSpace(apiKey)

	config := AIProviderConfig{
		Priority:    priority,
		Enabled:     true,
		MaxRetries:  2,
		EnvKey:      strings.ToUpper(providerName) + "_API_KEY",
		EndpointKey: strings.ToUpper(providerName) + "_ENDPOINT",
		ModelKey:    strings.ToUpper(providerName) + "_MODEL",
		BYOK:        true,
		Description: "Custom BYOK provider",
		GopassKey:   "terminal-ai/" + providerName + "_api_key",
	}

	providerConfig.Providers[providerName] = config

	providers[providerName] = AIProvider{
		Name:     providerName,
		APIKey:   apiKey,
		Endpoint: endpoint,
		Model:    model,
	}

	if err := saveProviderConfig(); err != nil {
		fmt.Printf("❌ Failed to add provider: %v\n", err)
		return
	}

	fmt.Printf("✅ Provider '%s' added successfully\n", providerName)
	fmt.Printf("   Priority: %d\n", config.Priority)
	fmt.Printf("   Endpoint: %s\n", endpoint)
	fmt.Printf("   Model: %s\n", model)
}

func setDefaultProvider(providerName string) {
	_, exists := providerConfig.Providers[providerName]
	if !exists {
		fmt.Printf("❌ Provider '%s' not found\n", providerName)
		return
	}

	providerConfig.DefaultProvider = providerName

	if err := saveProviderConfig(); err != nil {
		fmt.Printf("❌ Failed to set default provider: %v\n", err)
		return
	}

	fmt.Printf("✅ Default provider set to '%s'\n", providerName)
}

func saveProviderConfig() error {
	homeDir, _ := os.UserHomeDir()
	configFile := filepath.Join(homeDir, configDir, "providers.json")

	data, err := json.MarshalIndent(providerConfig, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configFile, data, 0644)
}

// BYOK CLI Commands

func handleBYOKCommand() {
	if len(os.Args) < 3 {
		showBYOKHelp()
		os.Exit(1)
	}

	subCmd := os.Args[2]

	switch subCmd {
	case "enable":
		toggleBYOKMode(true)
	case "disable":
		toggleBYOKMode(false)
	case "add":
		if len(os.Args) < 5 {
			fmt.Println("Usage: terminal-ai provider byok add <provider-name> <model-slug>")
			fmt.Println("Example: terminal-ai provider byok add SambaNova sambanova/llama-3.2")
			os.Exit(1)
		}
		addBYOKProviderCLI(os.Args[3], os.Args[4])
	case "remove":
		if len(os.Args) < 4 {
			fmt.Println("Usage: terminal-ai provider byok remove <provider-name>")
			fmt.Println("Example: terminal-ai provider byok remove SambaNova")
			os.Exit(1)
		}
		removeBYOKProviderCLI(os.Args[3])
	case "list":
		listBYOKProviders()
	case "order":
		if len(os.Args) < 4 {
			fmt.Println("Usage: terminal-ai provider byok order <provider1,provider2,provider3,...>")
			fmt.Println("Example: terminal-ai provider byok order Cerebras,SambaNova,Groq")
			os.Exit(1)
		}
		setBYOKProviderOrder(os.Args[3])
	case "test":
		testBYOKCLI()
	case "model":
		if len(os.Args) < 5 {
			fmt.Println("Usage: terminal-ai provider byok model <provider-name> <model-slug>")
			fmt.Println("Example: terminal-ai provider byok model Cerebras cerebras/llama-3.1-8b")
			os.Exit(1)
		}
		setBYOKModel(os.Args[3], os.Args[4])
	case "fallback":
		if len(os.Args) < 4 {
			fmt.Println("Usage: terminal-ai provider byok fallback <true|false>")
			fmt.Println("Example: terminal-ai provider byok fallback true")
			os.Exit(1)
		}
		toggleBYOKFallback(os.Args[3] == "true")
	default:
		showBYOKHelp()
	}
}

func toggleBYOKMode(enabled bool) {
	openrouterConfig, exists := providerConfig.Providers["openrouter"]
	if !exists {
		fmt.Println("❌ OpenRouter provider not found")
		return
	}

	if openrouterConfig.BYOKConfig == nil {
		// Initialize BYOK config with defaults
		openrouterConfig.BYOKConfig = &OpenRouterBYOKConfig{
			Enabled:               enabled,
			ProviderOrder:         []string{},
			AllowFallbackToShared: true,
			Models:                map[string]string{},
		}
	} else {
		openrouterConfig.BYOKConfig.Enabled = enabled
	}

	providerConfig.Providers["openrouter"] = openrouterConfig

	if err := saveProviderConfig(); err != nil {
		fmt.Printf("❌ Failed to %s BYOK: %v\n", map[bool]string{true: "enable", false: "disable"}[enabled], err)
		return
	}

	if enabled {
		fmt.Println("✅ BYOK mode enabled")
		fmt.Println("ℹ️  Add BYOK providers using: terminal-ai provider byok add <name> <model>")
	} else {
		fmt.Println("✅ BYOK mode disabled")
	}
}

func addBYOKProviderCLI(providerName, model string) {
	openrouterConfig, exists := providerConfig.Providers["openrouter"]
	if !exists {
		fmt.Println("❌ OpenRouter provider not found")
		return
	}

	if openrouterConfig.BYOKConfig == nil {
		fmt.Println("❌ BYOK not initialized. Enable BYOK first:")
		fmt.Println("   terminal-ai provider byok enable")
		return
	}

	// Check if provider already exists
	for _, existing := range openrouterConfig.BYOKConfig.ProviderOrder {
		if existing == providerName {
			fmt.Printf("❌ BYOK provider '%s' already exists\n", providerName)
			return
		}
	}

	// Add to order
	openrouterConfig.BYOKConfig.ProviderOrder = append(
		openrouterConfig.BYOKConfig.ProviderOrder,
		providerName,
	)

	// Add model
	if openrouterConfig.BYOKConfig.Models == nil {
		openrouterConfig.BYOKConfig.Models = make(map[string]string)
	}
	modelKey := normalizeProviderKeyCLI(providerName)
	openrouterConfig.BYOKConfig.Models[modelKey] = model

	providerConfig.Providers["openrouter"] = openrouterConfig

	if err := saveProviderConfig(); err != nil {
		fmt.Printf("❌ Failed to add BYOK provider: %v\n", err)
		return
	}

	fmt.Printf("✅ Added BYOK provider: %s\n", providerName)
	fmt.Printf("   Model: %s\n", model)
	fmt.Printf("ℹ️  Don't forget to add your API key at https://openrouter.ai/settings/integrations\n")
}

func removeBYOKProviderCLI(providerName string) {
	openrouterConfig, exists := providerConfig.Providers["openrouter"]
	if !exists || openrouterConfig.BYOKConfig == nil {
		fmt.Println("❌ BYOK not configured")
		return
	}

	// Remove from order
	newOrder := []string{}
	found := false
	for _, p := range openrouterConfig.BYOKConfig.ProviderOrder {
		if p != providerName {
			newOrder = append(newOrder, p)
		} else {
			found = true
		}
	}

	if !found {
		fmt.Printf("❌ BYOK provider '%s' not found\n", providerName)
		return
	}

	openrouterConfig.BYOKConfig.ProviderOrder = newOrder

	// Remove model
	modelKey := normalizeProviderKeyCLI(providerName)
	delete(openrouterConfig.BYOKConfig.Models, modelKey)

	providerConfig.Providers["openrouter"] = openrouterConfig

	if err := saveProviderConfig(); err != nil {
		fmt.Printf("❌ Failed to remove BYOK provider: %v\n", err)
		return
	}

	fmt.Printf("✅ Removed BYOK provider: %s\n", providerName)
}

func listBYOKProviders() {
	openrouterConfig, exists := providerConfig.Providers["openrouter"]
	if !exists || openrouterConfig.BYOKConfig == nil {
		fmt.Println("🔐 OpenRouter BYOK Configuration:")
		fmt.Println("   Status: Not configured")
		return
	}

	config := openrouterConfig.BYOKConfig

	fmt.Println("🔐 OpenRouter BYOK Configuration:")
	fmt.Println()

	status := "❌ Disabled"
	if config.Enabled {
		status = "✅ Enabled"
	}
	fmt.Printf("Status: %s\n", status)
	fmt.Printf("Fallback to Shared: %v\n", config.AllowFallbackToShared)
	fmt.Println()

	if len(config.ProviderOrder) == 0 {
		fmt.Println("No BYOK providers configured.")
		fmt.Println("Add providers using: terminal-ai provider byok add <name> <model>")
	} else {
		fmt.Println("BYOK Provider Priority Order:")
		for i, provider := range config.ProviderOrder {
			modelKey := normalizeProviderKeyCLI(provider)
			model := config.Models[modelKey]
			if model == "" {
				model = "(not set)"
			}
			fmt.Printf("  %d. %s\n", i+1, provider)
			fmt.Printf("     Model: %s\n", model)
		}
	}
	fmt.Println()
	fmt.Println("ℹ️  Available OpenRouter BYOK providers:")
	fmt.Println("   - Cerebras (cerebras/llama-3.1-8b)")
	fmt.Println("   - Google AI Studio (google/gemini-2.0-flash-exp:free)")
	fmt.Println("   - Groq (groq/llama-3.3-70b-versatile)")
	fmt.Println("   - SambaNova (sambanova/llama-3.2)")
	fmt.Println("   - z.ai (zai/llama-3.1)")
	fmt.Println("   - Together AI, Hyperbolic, Fireworks, etc.")
}

func setBYOKProviderOrder(orderStr string) {
	openrouterConfig, exists := providerConfig.Providers["openrouter"]
	if !exists || openrouterConfig.BYOKConfig == nil {
		fmt.Println("❌ BYOK not configured. Enable BYOK first:")
		fmt.Println("   terminal-ai provider byok enable")
		return
	}

	// Parse order string (comma-separated)
	newOrder := strings.Split(orderStr, ",")

	// Trim spaces
	for i := range newOrder {
		newOrder[i] = strings.TrimSpace(newOrder[i])
	}

	// Validate all providers exist
	for _, provider := range newOrder {
		found := false
		for _, existing := range openrouterConfig.BYOKConfig.ProviderOrder {
			if existing == provider {
				found = true
				break
			}
		}
		if !found {
			fmt.Printf("❌ BYOK provider '%s' not found. Add it first:\n", provider)
			fmt.Printf("   terminal-ai provider byok add %s <model>\n", provider)
			return
		}
	}

	openrouterConfig.BYOKConfig.ProviderOrder = newOrder
	providerConfig.Providers["openrouter"] = openrouterConfig

	if err := saveProviderConfig(); err != nil {
		fmt.Printf("❌ Failed to update BYOK order: %v\n", err)
		return
	}

	fmt.Println("✅ BYOK provider order updated")
	fmt.Println("New priority order:")
	for i, provider := range newOrder {
		fmt.Printf("  %d. %s\n", i+1, provider)
	}
}

func testBYOKCLI() {
	openrouterConfig, exists := providerConfig.Providers["openrouter"]
	if !exists || openrouterConfig.BYOKConfig == nil || !openrouterConfig.BYOKConfig.Enabled {
		fmt.Println("❌ BYOK not enabled. Enable it first:")
		fmt.Println("   terminal-ai provider byok enable")
		return
	}

	if len(openrouterConfig.BYOKConfig.ProviderOrder) == 0 {
		fmt.Println("❌ No BYOK providers configured. Add providers first:")
		fmt.Println("   terminal-ai provider byok add <name> <model>")
		return
	}

	provider, exists := providers["openrouter"]
	if !exists || provider.APIKey == "" {
		fmt.Println("❌ OpenRouter API key not configured")
		return
	}

	fmt.Println("🧪 Testing OpenRouter BYOK configuration...")
	fmt.Println()

	// Get first provider's model
	firstProvider := openrouterConfig.BYOKConfig.ProviderOrder[0]
	modelKey := normalizeProviderKeyCLI(firstProvider)
	testModel := openrouterConfig.BYOKConfig.Models[modelKey]
	if testModel == "" {
		testModel = provider.Model
	}

	req := Request{
		Model: testModel,
		Messages: []Message{
			{Role: "user", Content: "Hello! Say 'BYOK test successful' if you receive this."},
		},
		Stream: false,
	}

	openRouterReq := OpenRouterRequest{
		Model:    req.Model,
		Messages: req.Messages,
		Stream:   req.Stream,
		Provider: &OpenRouterProvider{
			AllowFallbacks: openrouterConfig.BYOKConfig.AllowFallbackToShared,
			Order:          openrouterConfig.BYOKConfig.ProviderOrder,
		},
	}

	reqBody, _ := json.Marshal(openRouterReq)

	client := &http.Client{Timeout: 30 * time.Second}
	httpReq, _ := http.NewRequest("POST", provider.Endpoint, strings.NewReader(string(reqBody)))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+provider.APIKey)
	httpReq.Header.Set("HTTP-Referer", "https://terminal-ai.local")
	httpReq.Header.Set("X-Title", "Terminal AI CLI")

	fmt.Printf("🔄 Testing with BYOK order: %v\n", openrouterConfig.BYOKConfig.ProviderOrder)
	fmt.Println()

	resp, err := client.Do(httpReq)
	if err != nil {
		fmt.Printf("❌ Test failed: %v\n", err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var response Response
	json.Unmarshal(body, &response)

	if response.Error != nil {
		fmt.Printf("❌ API Error: %s\n", response.Error.Message)
		return
	}

	if len(response.Choices) > 0 {
		fmt.Println("✅ BYOK test successful!")
		fmt.Printf("Response: %s\n", response.Choices[0].Message.Content)

		// Show which providers are configured
		fmt.Println()
		fmt.Println("Configured BYOK providers:")
		for i, p := range openrouterConfig.BYOKConfig.ProviderOrder {
			fmt.Printf("  %d. %s\n", i+1, p)
		}
	} else {
		fmt.Println("❌ No response received")
	}
}

func setBYOKModel(providerName, model string) {
	openrouterConfig, exists := providerConfig.Providers["openrouter"]
	if !exists || openrouterConfig.BYOKConfig == nil {
		fmt.Println("❌ BYOK not configured")
		return
	}

	// Check if provider exists
	found := false
	for _, existing := range openrouterConfig.BYOKConfig.ProviderOrder {
		if existing == providerName {
			found = true
			break
		}
	}

	if !found {
		fmt.Printf("❌ BYOK provider '%s' not found. Add it first:\n", providerName)
		fmt.Printf("   terminal-ai provider byok add %s <model>\n", providerName)
		return
	}

	// Update model
	if openrouterConfig.BYOKConfig.Models == nil {
		openrouterConfig.BYOKConfig.Models = make(map[string]string)
	}
	modelKey := normalizeProviderKeyCLI(providerName)
	openrouterConfig.BYOKConfig.Models[modelKey] = model

	providerConfig.Providers["openrouter"] = openrouterConfig

	if err := saveProviderConfig(); err != nil {
		fmt.Printf("❌ Failed to update model: %v\n", err)
		return
	}

	fmt.Printf("✅ Updated model for %s: %s\n", providerName, model)
}

func toggleBYOKFallback(enabled bool) {
	openrouterConfig, exists := providerConfig.Providers["openrouter"]
	if !exists || openrouterConfig.BYOKConfig == nil {
		fmt.Println("❌ BYOK not configured")
		return
	}

	openrouterConfig.BYOKConfig.AllowFallbackToShared = enabled
	providerConfig.Providers["openrouter"] = openrouterConfig

	if err := saveProviderConfig(); err != nil {
		fmt.Printf("❌ Failed to update fallback setting: %v\n", err)
		return
	}

	if enabled {
		fmt.Println("✅ Fallback to OpenRouter shared enabled")
	} else {
		fmt.Println("✅ Fallback to OpenRouter shared disabled")
		fmt.Println("⚠️  If all BYOK providers fail, requests will fail without fallback")
	}
}

func normalizeProviderKeyCLI(name string) string {
	// Convert to lowercase and replace spaces/special chars with underscores
	key := strings.ToLower(name)
	key = strings.ReplaceAll(key, " ", "_")
	key = strings.ReplaceAll(key, "-", "_")
	// Remove any remaining non-alphanumeric characters
	var result strings.Builder
	for _, r := range key {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			result.WriteRune(r)
		}
	}
	return result.String()
}

func showBYOKHelp() {
	fmt.Println("OpenRouter BYOK (Bring Your Own Key) Commands:")
	fmt.Println()
	fmt.Println("  terminal-ai provider byok enable                - Enable BYOK mode")
	fmt.Println("  terminal-ai provider byok disable               - Disable BYOK mode")
	fmt.Println("  terminal-ai provider byok add <name> <model>    - Add a BYOK provider")
	fmt.Println("  terminal-ai provider byok remove <name>         - Remove a BYOK provider")
	fmt.Println("  terminal-ai provider byok list                  - List configured BYOK providers")
	fmt.Println("  terminal-ai provider byok order <p1,p2,p3...>   - Set provider priority order")
	fmt.Println("  terminal-ai provider byok test                  - Test BYOK configuration")
	fmt.Println("  terminal-ai provider byok model <name> <model>  - Set model for a provider")
	fmt.Println("  terminal-ai provider byok fallback <true|false> - Toggle fallback to shared")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  terminal-ai provider byok enable")
	fmt.Println("  terminal-ai provider byok add SambaNova sambanova/llama-3.2")
	fmt.Println("  terminal-ai provider byok add z.ai zai/llama-3.1")
	fmt.Println("  terminal-ai provider byok order Cerebras,SambaNova,Groq")
	fmt.Println("  terminal-ai provider byok test")
	fmt.Println()
	fmt.Println("Popular BYOK Providers:")
	fmt.Println("  - Cerebras: cerebras/llama-3.1-8b")
	fmt.Println("  - Google AI Studio: google/gemini-2.0-flash-exp:free")
	fmt.Println("  - Groq: groq/llama-3.3-70b-versatile")
	fmt.Println("  - SambaNova: sambanova/llama-3.2")
	fmt.Println("  - z.ai: zai/llama-3.1")
	fmt.Println("  - Together AI, Hyperbolic, Fireworks, etc.")
	fmt.Println()
	fmt.Println("Note: Add your API keys at https://openrouter.ai/settings/integrations")
}

func showProviderHelp() {
	fmt.Println("Provider Management Commands:")
	fmt.Println()
	fmt.Println("  terminal-ai provider list                      - List all providers with configuration")
	fmt.Println("  terminal-ai provider test <provider>           - Test a specific provider")
	fmt.Println("  terminal-ai provider enable <provider>         - Enable a provider")
	fmt.Println("  terminal-ai provider disable <provider>        - Disable a provider")
	fmt.Println("  terminal-ai provider priority <provider> <n>   - Set provider priority (0=highest)")
	fmt.Println("  terminal-ai provider add <provider>            - Add a new custom provider")
	fmt.Println("  terminal-ai provider default <provider>        - Set default provider")
	fmt.Println()
	fmt.Println("OpenRouter BYOK Commands:")
	fmt.Println("  terminal-ai provider byok enable               - Enable BYOK mode")
	fmt.Println("  terminal-ai provider byok add <name> <model>   - Add a BYOK provider (SambaNova, z.ai, etc.)")
	fmt.Println("  terminal-ai provider byok remove <name>        - Remove a BYOK provider")
	fmt.Println("  terminal-ai provider byok list                 - List configured BYOK providers")
	fmt.Println("  terminal-ai provider byok order <p1,p2...>     - Set provider priority order")
	fmt.Println("  terminal-ai provider byok test                 - Test BYOK configuration")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  terminal-ai provider list")
	fmt.Println("  terminal-ai provider test openrouter")
	fmt.Println("  terminal-ai provider byok enable")
	fmt.Println("  terminal-ai provider byok add SambaNova sambanova/llama-3.2")
	fmt.Println("  terminal-ai provider byok order Cerebras,SambaNova,Groq")
	fmt.Println("  terminal-ai provider byok test")
}

func handleChatCommand() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: terminal-ai chat --list | chat --new [message] | chat --last [message] | chat --session <id> [message]")
		os.Exit(1)
	}

	flag := os.Args[2]

	switch flag {
	case "--list":
		listSessionsCLI()
	case "--new":
		var message string
		if len(os.Args) > 3 {
			message = strings.Join(os.Args[3:], " ")
		}
		startNewSession(message)
	case "--last":
		var message string
		if len(os.Args) > 3 {
			message = strings.Join(os.Args[3:], " ")
		}
		startLastSession(message)
	case "--session":
		if len(os.Args) < 4 {
			fmt.Println("Usage: terminal-ai chat --session <id> [message]")
			os.Exit(1)
		}
		sessionID := os.Args[3]
		var message string
		if len(os.Args) > 4 {
			message = strings.Join(os.Args[4:], " ")
		}
		startSession(sessionID, message)
	default:
		fmt.Println("Unknown chat flag. Use: --list | --new | --last | --session")
	}
}

func handleHistoryCommand() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: terminal-ai history list | history view <id> | history export <id> [filename] [--format txt|md] | history delete <id> | history clear")
		os.Exit(1)
	}

	subCmd := os.Args[2]

	switch subCmd {
	case "list":
		listSessionsCLI()
	case "view":
		if len(os.Args) < 4 {
			fmt.Println("Usage: terminal-ai history view <id>")
			os.Exit(1)
		}
		viewSessionCLI(os.Args[3])
	case "export":
		if len(os.Args) < 4 {
			fmt.Println("Usage: terminal-ai history export <id> [filename] [--format txt|md]")
			os.Exit(1)
		}
		sessionID := os.Args[3]
		filename := ""
		format := "txt"
		for i := 4; i < len(os.Args); i++ {
			if os.Args[i] == "--format" && i+1 < len(os.Args) {
				format = os.Args[i+1]
				i++
			} else if filename == "" {
				filename = os.Args[i]
			}
		}
		exportSession(sessionID, filename, format)
	case "delete":
		if len(os.Args) < 4 {
			fmt.Println("Usage: terminal-ai history delete <id>")
			os.Exit(1)
		}
		deleteSessionCLI(os.Args[3])
	case "clear":
		clearHistoryCLI()
	default:
		fmt.Println("Unknown history command. Use: list | view | export | delete | clear")
	}
}

func listSessionsCLI() {
	sessions := listSessions()
	if len(sessions) == 0 {
		fmt.Println("📚 No chat sessions found")
		return
	}

	fmt.Printf("📚 Chat History:\n\n")
	for i, session := range sessions {
		fmt.Printf("%d. %s\n", i+1, session.Title)
		fmt.Printf("   ID: %s\n", session.ID)
		fmt.Printf("   Provider: %s\n", session.Provider)
		fmt.Printf("   Messages: %d\n", len(session.Messages))
		fmt.Printf("   Created: %s\n", session.CreatedAt)
		fmt.Printf("   Updated: %s\n\n", session.UpdatedAt)
	}
}

func viewSessionCLI(sessionID string) {
	session, err := getSession(sessionID)
	if err != nil {
		fmt.Printf("❌ Session not found: %s\n", sessionID)
		return
	}

	fmt.Printf("📖 %s\n", session.Title)
	fmt.Printf("   ID: %s\n", session.ID)
	fmt.Printf("   Provider: %s\n", session.Provider)
	fmt.Printf("   Created: %s\n\n", session.CreatedAt)

	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	for _, msg := range session.Messages {
		if msg.Role == "user" {
			fmt.Printf("\n👤 User:\n%s\n", msg.Content)
		} else {
			fmt.Printf("\n🤖 AI:\n%s\n", msg.Content)
		}
	}

	fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
}

func startNewSession(message string) {
	fmt.Println("✨ Starting new chat session")
	startREPLWithSession(nil, message)
}

func startLastSession(message string) {
	session := getLatestSession()
	if session == nil {
		fmt.Println("⚠️  No previous session found. Starting new chat.")
		startREPLWithSession(nil, message)
		return
	}
	startREPLWithSession(session, message)
}

func startSession(sessionID, message string) {
	session, err := getSession(sessionID)
	if err != nil {
		fmt.Printf("❌ Session not found: %s\n", sessionID)
		return
	}
	startREPLWithSession(session, message)
}

func startREPLWithSession(session *ChatSession, initialMessage string) {
	providerName := providerConfig.DefaultProvider

	if session == nil {
		if initialMessage == "" {
			fmt.Print("Your message: ")
			reader := bufio.NewReader(os.Stdin)
			msg, _ := reader.ReadString('\n')
			msg = strings.TrimSpace(msg)
			if msg == "" {
				fmt.Println("Message cannot be empty")
				return
			}
			initialMessage = msg
		}

		fmt.Printf("🎯 Primary provider: %s\n", providerName)
		fmt.Printf("🔄 Fallback enabled: %v\n", providerConfig.FallbackEnabled)

		session = createSession(truncateTitle(initialMessage), providerName, "user")
		if initialMessage != "" {
			updateSession(session.ID, "user", initialMessage)
		}
	} else {
		fmt.Printf("📂 Loaded session: %s\n", session.Title)
		fmt.Printf("   Messages: %d\n", len(session.Messages))
		fmt.Printf("   Provider: %s\n\n", session.Provider)
		providerName = session.Provider
	}

	if initialMessage != "" && len(session.Messages) == 0 {
		sessionWithHistory(session, providerName, initialMessage)
	}

	for {
		fmt.Print("\nContinue? (y/n): ")
		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(answer)

		if strings.ToLower(answer) != "y" {
			fmt.Printf("\n💾 Chat saved with ID: %s\n", session.ID)
			return
		}

		fmt.Print("Your message: ")
		msg, _ := reader.ReadString('\n')
		msg = strings.TrimSpace(msg)

		if msg == "" {
			continue
		}

		sessionWithHistory(session, providerName, msg)
	}
}

func sessionWithHistory(session *ChatSession, providerName, message string) {
	messages := []Message{{Role: "user", Content: message}}
	for _, msg := range session.Messages {
		if msg.Role == "user" || msg.Role == "assistant" {
			messages = append(messages, Message{Role: msg.Role, Content: msg.Content})
		}
	}

	skills := findMatchingSkills(message)
	finalMessage := message

	if len(skills) > 0 {
		for _, skill := range skills {
			finalMessage = skill.Template + "\n\n" + finalMessage
		}
	}

	results := searchRAGWithFilters(message, "user", "")
	if len(results) > 0 {
		context := "\n\nRelevant documents:\n"
		for _, doc := range results {
			contentLen := len(doc.Content)
			if contentLen > 200 {
				contentLen = 200
			}
			context += fmt.Sprintf("- %s: %s\n", doc.Path, doc.Content[:contentLen])
		}
		finalMessage += context
	}

	provider := providers[providerName]

	req := Request{
		Model:    provider.Model,
		Messages: messages,
		Stream:   streamingEnabled,
	}

	var response *Response
	var actualProvider string
	var err error
	var streamingErr error
	var fullResponse string

	if streamingEnabled {
		// Use streaming mode
		if providerConfig.FallbackEnabled {
			fmt.Printf("🔄 Fallback enabled: %v\n", providerConfig.FallbackEnabled)
		}
		fmt.Println("📝 Response (streaming):")

		// For chat sessions with history, we need to capture the full response
		// We'll use a modified approach that captures output for saving to history
		streamingErr = makeStreamingRequestWithCapture(provider.Endpoint, provider.APIKey, req, provider.Name, &fullResponse)
		actualProvider = providerName

		if streamingErr != nil {
			fmt.Printf("\n❌ Streaming Error: %v\n", streamingErr)
			return
		}

		if fullResponse != "" {
			updateSession(session.ID, "assistant", fullResponse)
		}
	} else {
		// Use non-streaming mode
		if providerConfig.FallbackEnabled {
			response, actualProvider, err = makeRequestWithFallback(
				provider.Endpoint, provider.APIKey, req, providerName,
			)
		} else {
			response, err = makeRequest(provider.Endpoint, provider.APIKey, req, provider.Name)
			actualProvider = providerName
		}

		if err != nil {
			fmt.Printf("❌ Error: %v\n", err)
			return
		}

		if response.Error != nil {
			fmt.Printf("❌ API Error: %s\n", response.Error.Message)
			return
		}

		if len(response.Choices) > 0 {
			if actualProvider != providerName {
				fmt.Printf("📡 Response from fallback provider: %s\n", actualProvider)
			} else {
				fmt.Printf("✅ Success with provider: %s\n", actualProvider)
			}
			fmt.Println(response.Choices[0].Message.Content)
			updateSession(session.ID, "assistant", response.Choices[0].Message.Content)
		}
	}
}

func deleteSessionCLI(sessionID string) {
	fmt.Printf("⚠️  Delete session '%s'? (y/n): ", sessionID)
	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(answer)

	if strings.ToLower(answer) == "y" {
		if err := deleteSession(sessionID); err != nil {
			fmt.Printf("❌ Failed to delete session: %v\n", err)
		} else {
			fmt.Printf("✅ Session '%s' deleted\n", sessionID)
		}
	}
}

func clearHistoryCLI() {
	sessions := listSessions()
	fmt.Printf("⚠️  This will delete all %d chat sessions. Continue? (y/n): ", len(sessions))
	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(answer)

	if strings.ToLower(answer) == "y" {
		if err := clearAllHistory(); err != nil {
			fmt.Printf("❌ Failed to clear history: %v\n", err)
		} else {
			fmt.Println("✅ Chat history cleared")
		}
	}
}

func exportSession(sessionID, filename, format string) {
	session, err := getSession(sessionID)
	if err != nil {
		fmt.Printf("❌ Session not found: %s\n", sessionID)
		return
	}

	if filename == "" {
		if format == "md" {
			filename = fmt.Sprintf("%s.md", strings.Map(func(r rune) rune {
				if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
					return r
				}
				return '-'
			}, session.Title[:min(30, len(session.Title))]))
		} else {
			filename = fmt.Sprintf("%s.txt", strings.Map(func(r rune) rune {
				if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
					return r
				}
				return '-'
			}, session.Title[:min(30, len(session.Title))]))
		}
	}

	var content string

	if format == "md" {
		content = fmt.Sprintf("# %s\n\n**ID:** %s\n**Provider:** %s\n**Created:** %s\n\n---\n\n## Conversation\n\n",
			session.Title, session.ID, session.Provider, session.CreatedAt)

		for _, msg := range session.Messages {
			if msg.Role == "user" {
				content += fmt.Sprintf("### User\n%s\n\n", msg.Content)
			} else {
				content += fmt.Sprintf("### Assistant\n%s\n\n", msg.Content)
			}
		}
	} else {
		content = fmt.Sprintf("Title: %s\nID: %s\nProvider: %s\nCreated: %s\n\n%s\n\n",
			session.Title, session.ID, session.Provider, session.CreatedAt, strings.Repeat("=", 60))

		for _, msg := range session.Messages {
			if msg.Role == "user" {
				content += fmt.Sprintf("[User] %s\n", msg.Content)
			} else {
				content += fmt.Sprintf("[AI] %s\n", msg.Content)
			}
		}
	}

	if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
		fmt.Printf("❌ Failed to export session: %v\n", err)
	} else {
		fmt.Printf("✅ Session exported to: %s\n", filename)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func chatWithAI(providerName, message string) {
	if providerName == "" {
		providerName = providerConfig.DefaultProvider
	}

	provider, exists := providers[providerName]
	if !exists {
		fmt.Printf("Unknown provider: %s\n", providerName)
		os.Exit(1)
	}

	if provider.APIKey == "" {
		fmt.Printf("API key not configured for %s\n", providerName)
		os.Exit(1)
	}

	skills := findMatchingSkills(message)
	finalMessage := message

	if len(skills) > 0 {
		for _, skill := range skills {
			finalMessage = skill.Template + "\n\n" + finalMessage
		}
	}

	results := searchRAGWithFilters(message, "user", "")
	if len(results) > 0 {
		context := "\n\nRelevant documents:\n"
		for _, doc := range results {
			contentLen := len(doc.Content)
			if contentLen > 200 {
				contentLen = 200
			}
			context += fmt.Sprintf("- %s: %s\n", doc.Path, doc.Content[:contentLen])
		}
		finalMessage += context
	}

	req := Request{
		Model: provider.Model,
		Messages: []Message{
			{Role: "user", Content: finalMessage},
		},
		Stream: streamingEnabled,
	}

	var response *Response
	var actualProvider string
	var err error
	var streamingErr error

	if streamingEnabled {
		// Use streaming mode
		fmt.Printf("🎯 Provider: %s\n", providerName)
		if providerConfig.FallbackEnabled {
			fmt.Printf("🔄 Fallback enabled: %v\n", providerConfig.FallbackEnabled)
		}
		fmt.Println("📝 Response (streaming):")

		streamingErr = makeStreamingRequest(provider.Endpoint, provider.APIKey, req, provider.Name)
		actualProvider = providerName

		if streamingErr != nil {
			fmt.Printf("\n❌ Streaming Error: %v\n", streamingErr)
			return
		}
	} else {
		// Use non-streaming mode
		if providerConfig.FallbackEnabled {
			fmt.Printf("🎯 Primary provider: %s\n", providerName)
			fmt.Printf("🔄 Fallback enabled: %v\n", providerConfig.FallbackEnabled)
			response, actualProvider, err = makeRequestWithFallback(
				provider.Endpoint, provider.APIKey, req, providerName,
			)
		} else {
			response, err = makeRequest(provider.Endpoint, provider.APIKey, req, provider.Name)
			actualProvider = providerName
		}

		if err != nil {
			fmt.Printf("❌ Error: %v\n", err)
			return
		}

		if response.Error != nil {
			fmt.Printf("❌ API Error: %s\n", response.Error.Message)
			return
		}

		if len(response.Choices) > 0 {
			if actualProvider != providerName {
				fmt.Printf("📡 Response from fallback provider: %s\n", actualProvider)
			}
			fmt.Println(response.Choices[0].Message.Content)
		}
	}

	fmt.Print("\nContinue? (y/n): ")
	var answer string
	fmt.Scanln(&answer)
	if strings.ToLower(answer) != "y" {
		return
	}

	fmt.Print("Your message: ")
	reader := bufio.NewReader(os.Stdin)
	msg, _ := reader.ReadString('\n')
	msg = strings.TrimSpace(msg)

	if msg != "" {
		chatWithAI(actualProvider, msg)
	}
}

func findMatchingSkills(message string) []Skill {
	homeDir, _ := os.UserHomeDir()
	skillsDir := filepath.Join(homeDir, configDir, "skills")

	var matches []Skill

	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return matches
	}

	for _, entry := range entries {
		if entry.IsDir() {
			skillFile := filepath.Join(skillsDir, entry.Name(), "skill.json")
			data, err := os.ReadFile(skillFile)
			if err == nil {
				var skill Skill
				json.Unmarshal(data, &skill)

				for _, trigger := range skill.Triggers {
					if strings.Contains(strings.ToLower(message), strings.ToLower(trigger)) {
						matches = append(matches, skill)
						break
					}
				}
			}
		}
	}

	return matches
}

func makeRequestWithFallback(endpoint, apiKey string, req Request, providerName string) (*Response, string, error) {
	var lastError error
	attemptedProviders := make(map[string]bool)

	orderedProviders := getOrderedProviders()

	for _, providerName := range orderedProviders {
		if attemptedProviders[providerName] {
			continue
		}

		attemptedProviders[providerName] = true

		config := providerConfig.Providers[providerName]
		if !config.Enabled {
			continue
		}

		provider := providers[providerName]
		if provider.APIKey == "" {
			fmt.Printf("⚠️  Provider '%s' has no API key, skipping...\n", providerName)
			continue
		}

		fmt.Printf("🔄 Attempting provider: %s (Priority %d)\n", providerName, config.Priority)

		var response *Response
		var err error

		for retry := 0; retry <= config.MaxRetries; retry++ {
			if retry > 0 {
				fmt.Printf("   Retry %d/%d...\n", retry, config.MaxRetries)
				time.Sleep(time.Duration(providerConfig.RetryDelayMs) * time.Millisecond)
			}

			response, err = makeRequest(provider.Endpoint, provider.APIKey, req, provider.Name)

			if err == nil && (response.Error == nil || response.Error.Message == "") {
				fmt.Printf("✅ Success with provider: %s\n", providerName)
				return response, providerName, nil
			}

			errorType := classifyError(err, response)
			lastError = fmt.Errorf("provider %s: %w", providerName, combineErrors(err, response))

			if errorType == "rate_limit" {
				fmt.Printf("   ⚠️  Rate limit exceeded on %s\n", providerName)
				if retry < config.MaxRetries {
					continue
				}
				break
			} else if errorType == "server_error" || errorType == "network" {
				fmt.Printf("   ⚠️  %s error on %s\n", errorType, providerName)
				if retry < config.MaxRetries {
					continue
				}
				break
			} else if errorType == "timeout" {
				fmt.Printf("   ⚠️  Timeout on %s\n", providerName)
				if retry < config.MaxRetries {
					continue
				}
				break
			}
		}
	}

	return nil, "", fmt.Errorf("all providers failed. Last error: %w", lastError)
}

func makeRequest(endpoint, apiKey string, req Request, provider string) (*Response, error) {
	var reqBody []byte
	var err error

	// Check if OpenRouter with BYOK enabled
	if provider == "openrouter" {
		if config, exists := providerConfig.Providers["openrouter"]; exists && config.BYOKConfig != nil && config.BYOKConfig.Enabled {
			// Build OpenRouter request with BYOK provider ordering
			openRouterReq := OpenRouterRequest{
				Model:    req.Model,
				Messages: req.Messages,
				Stream:   req.Stream,
				Provider: &OpenRouterProvider{
					AllowFallbacks: config.BYOKConfig.AllowFallbackToShared,
					Order:          config.BYOKConfig.ProviderOrder,
				},
			}
			reqBody, err = json.Marshal(openRouterReq)
			if err != nil {
				return nil, err
			}
			fmt.Printf("🔄 Using OpenRouter BYOK with order: %v\n", config.BYOKConfig.ProviderOrder)
		} else {
			// Regular OpenRouter request without BYOK
			reqBody, err = json.Marshal(req)
			if err != nil {
				return nil, err
			}
		}
	} else {
		// Non-OpenRouter providers
		reqBody, err = json.Marshal(req)
		if err != nil {
			return nil, err
		}
	}

	client := &http.Client{Timeout: 120 * time.Second}

	httpReq, err := http.NewRequest("POST", endpoint, strings.NewReader(string(reqBody)))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")

	if provider == "openrouter" {
		httpReq.Header.Set("Authorization", "Bearer "+apiKey)
		httpReq.Header.Set("HTTP-Referer", "https://terminal-ai.local")
		httpReq.Header.Set("X-Title", "Terminal AI CLI")
	} else if provider == "gemini" {
		httpReq.Header.Set("x-goog-api-key", apiKey)
	} else {
		httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var response Response
	json.Unmarshal(body, &response)

	return &response, nil
}

func makeStreamingRequest(endpoint, apiKey string, req Request, provider string) error {
	var reqBody []byte
	var err error

	// Check if OpenRouter with BYOK enabled
	if provider == "openrouter" {
		if config, exists := providerConfig.Providers["openrouter"]; exists && config.BYOKConfig != nil && config.BYOKConfig.Enabled {
			// Build OpenRouter request with BYOK provider ordering
			openRouterReq := OpenRouterRequest{
				Model:    req.Model,
				Messages: req.Messages,
				Stream:   true,
				Provider: &OpenRouterProvider{
					AllowFallbacks: config.BYOKConfig.AllowFallbackToShared,
					Order:          config.BYOKConfig.ProviderOrder,
				},
			}
			reqBody, err = json.Marshal(openRouterReq)
			if err != nil {
				return err
			}
			fmt.Printf("🔄 Using OpenRouter BYOK with order: %v\n", config.BYOKConfig.ProviderOrder)
		} else {
			// Regular OpenRouter request without BYOK
			reqBody, err = json.Marshal(req)
			if err != nil {
				return err
			}
		}
	} else {
		// Non-OpenRouter providers
		reqBody, err = json.Marshal(req)
		if err != nil {
			return err
		}
	}

	client := &http.Client{Timeout: 120 * time.Second}

	httpReq, err := http.NewRequest("POST", endpoint, strings.NewReader(string(reqBody)))
	if err != nil {
		return err
	}

	httpReq.Header.Set("Content-Type", "application/json")

	if provider == "openrouter" {
		httpReq.Header.Set("Authorization", "Bearer "+apiKey)
		httpReq.Header.Set("HTTP-Referer", "https://terminal-ai.local")
		httpReq.Header.Set("X-Title", "Terminal AI CLI")
	} else if provider == "gemini" {
		httpReq.Header.Set("x-goog-api-key", apiKey)
	} else {
		httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Check for SSE data prefix
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")

		// Check for stream end
		if data == "[DONE]" {
			break
		}

		// Parse the streaming response
		var streamResp StreamingResponse
		if err := json.Unmarshal([]byte(data), &streamResp); err != nil {
			// Some providers might send different formats, skip unparseable lines
			continue
		}

		// Check for API errors in stream
		if streamResp.Error != nil {
			return fmt.Errorf("API Error: %s", streamResp.Error.Message)
		}

		// Extract and print content
		if len(streamResp.Choices) > 0 {
			content := streamResp.Choices[0].Delta.Content
			if content != "" {
				fmt.Print(content)
			}
		}
	}

	fmt.Println() // Add newline at the end
	return nil
}

func makeStreamingRequestWithCapture(endpoint, apiKey string, req Request, provider string, capture *string) error {
	var reqBody []byte
	var err error

	// Check if OpenRouter with BYOK enabled
	if provider == "openrouter" {
		if config, exists := providerConfig.Providers["openrouter"]; exists && config.BYOKConfig != nil && config.BYOKConfig.Enabled {
			// Build OpenRouter request with BYOK provider ordering
			openRouterReq := OpenRouterRequest{
				Model:    req.Model,
				Messages: req.Messages,
				Stream:   true,
				Provider: &OpenRouterProvider{
					AllowFallbacks: config.BYOKConfig.AllowFallbackToShared,
					Order:          config.BYOKConfig.ProviderOrder,
				},
			}
			reqBody, err = json.Marshal(openRouterReq)
			if err != nil {
				return err
			}
			fmt.Printf("🔄 Using OpenRouter BYOK with order: %v\n", config.BYOKConfig.ProviderOrder)
		} else {
			// Regular OpenRouter request without BYOK
			reqBody, err = json.Marshal(req)
			if err != nil {
				return err
			}
		}
	} else {
		// Non-OpenRouter providers
		reqBody, err = json.Marshal(req)
		if err != nil {
			return err
		}
	}

	client := &http.Client{Timeout: 120 * time.Second}

	httpReq, err := http.NewRequest("POST", endpoint, strings.NewReader(string(reqBody)))
	if err != nil {
		return err
	}

	httpReq.Header.Set("Content-Type", "application/json")

	if provider == "openrouter" {
		httpReq.Header.Set("Authorization", "Bearer "+apiKey)
		httpReq.Header.Set("HTTP-Referer", "https://terminal-ai.local")
		httpReq.Header.Set("X-Title", "Terminal AI CLI")
	} else if provider == "gemini" {
		httpReq.Header.Set("x-goog-api-key", apiKey)
	} else {
		httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var capturedContent strings.Builder
	reader := bufio.NewReader(resp.Body)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Check for SSE data prefix
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")

		// Check for stream end
		if data == "[DONE]" {
			break
		}

		// Parse the streaming response
		var streamResp StreamingResponse
		if err := json.Unmarshal([]byte(data), &streamResp); err != nil {
			// Some providers might send different formats, skip unparseable lines
			continue
		}

		// Check for API errors in stream
		if streamResp.Error != nil {
			return fmt.Errorf("API Error: %s", streamResp.Error.Message)
		}

		// Extract and print content
		if len(streamResp.Choices) > 0 {
			content := streamResp.Choices[0].Delta.Content
			if content != "" {
				fmt.Print(content)
				capturedContent.WriteString(content)
			}
		}
	}

	*capture = capturedContent.String()
	fmt.Println() // Add newline at the end
	return nil
}

func showHelp() {
	fmt.Println("Terminal AI CLI - AI Assistant with Web, Skills, RAG & Chat History")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  terminal-ai [provider] <message>       - Chat with AI")
	fmt.Println("  terminal-ai chat --list/--new/--last/--session <id>  - Chat sessions")
	fmt.Println("  terminal-ai history list/view/export/delete <id>/clear  - Chat history")
	fmt.Println("  terminal-ai rag index <dir> / search <query>  - Local RAG")
	fmt.Println("  terminal-ai skill list/create <name>   - Custom skills")
	fmt.Println("  terminal-ai user list/create/delete    - User management")
	fmt.Println("  terminal-ai provider list/test/enable/disable/priority/add/default  - Provider config")
	fmt.Println("  terminal-ai web <url> / web-server      - Web fetch & server")
	fmt.Println("  terminal-ai --help                     - Show this help")
	fmt.Println()
	fmt.Println("Providers (default: openrouter):")
	fmt.Println("  - openrouter (1) - gemini (2) - groq (3) - Custom BYOK (0+)")
}
