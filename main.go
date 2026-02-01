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
	Priority    int    `json:"priority"`
	Enabled     bool   `json:"enabled"`
	MaxRetries  int    `json:"max_retries"`
	GopassKey   string `json:"gopass_key"`
	EnvKey      string `json:"env_key"`
	EndpointKey string `json:"endpoint_key"`
	ModelKey    string `json:"model_key"`
	BYOK        bool   `json:"byok"`
	Description string `json:"description"`
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

type APIError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

type RAGDocument struct {
	Path      string   `json:"path"`
	Content   string   `json:"content"`
	Keywords  []string `json:"keywords"`
	IndexedAt string   `json:"indexed_at"`
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

var ragIndex RAGIndex
var providers map[string]AIProvider
var useGopass bool
var providerConfig ProviderGlobalConfig

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

	if err := loadProviderConfig(); err != nil {
		fmt.Printf("Warning: Failed to load provider config: %v\n", err)
	}

	initProviders()
	securityMgr = initSecurityManager()
	loadRAGIndex()

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
			Path:      path,
			Content:   string(content),
			Keywords:  keywords,
			IndexedAt: time.Now().Format(time.RFC3339),
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

	fmt.Printf("✅ Indexed %d documents\n", count)
}

func searchRAG(query string) []RAGDocument {
	queryWords := tokenize(query)
	type scoreDoc struct {
		doc   RAGDocument
		score int
	}
	var scored []scoreDoc

	for _, doc := range ragIndex.Documents {
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
	fmt.Println("Examples:")
	fmt.Println("  terminal-ai provider list")
	fmt.Println("  terminal-ai provider test openrouter")
	fmt.Println("  terminal-ai provider enable gemini")
	fmt.Println("  terminal-ai provider priority openrouter_custom 0")
	fmt.Println("  terminal-ai provider add myprovider")
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

	results := searchRAG(message)
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
		Stream: false,
	}

	var response *Response
	var actualProvider string
	var err error

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
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, err
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

func showHelp() {
	fmt.Println("Terminal AI CLI - AI Assistant with Web, Skills & RAG")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  terminal-ai [provider] <message>       - Chat with AI")
	fmt.Println("  terminal-ai web <url>                  - Fetch web content")
	fmt.Println("  terminal-ai rag index <dir>             - Index local files")
	fmt.Println("  terminal-ai rag search <query>          - Search indexed files")
	fmt.Println("  terminal-ai skill create <name>         - Create a skill")
	fmt.Println("  terminal-ai skill list                  - List all skills")
	fmt.Println("  terminal-ai user create <name> <role>   - Create user")
	fmt.Println("  terminal-ai user list                   - List users")
	fmt.Println("  terminal-ai user delete <name>          - Delete user")
	fmt.Println("  terminal-ai provider list               - List providers with fallback config")
	fmt.Println("  terminal-ai provider test <provider>    - Test a specific provider")
	fmt.Println("  terminal-ai provider enable/disable      - Enable/disable a provider")
	fmt.Println("  terminal-ai provider priority <p> <n>    - Set provider priority")
	fmt.Println("  terminal-ai provider add <name>         - Add custom BYOK provider")
	fmt.Println("  terminal-ai provider default <provider>  - Set default provider")
	fmt.Println("  terminal-ai web-server                  - Start web server")
	fmt.Println("  terminal-ai --help                       - Show this help")
	fmt.Println()
	fmt.Println("Providers (default: openrouter):")
	fmt.Println("  - openrouter (priority 1)")
	fmt.Println("  - gemini (priority 2)")
	fmt.Println("  - groq (priority 3)")
	fmt.Println("  - Custom BYOK providers (priority 0+)")
	fmt.Println()
	fmt.Println("Documentation:")
	fmt.Println("  docs/QUICKSTART.md         - Quick start guide")
	fmt.Println("  docs/SYSTEM.md             - Full system documentation")
	fmt.Println("  docs/PROVIDER_CONFIG.md    - Provider fallback & BYOK guide")
	fmt.Println("  docs/SECURITY.md          - Security guide")
	fmt.Println("  docs/RASPBERRY_PI.md      - Raspberry Pi deployment")
}
