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

func main() {
	homeDir, _ := os.UserHomeDir()
	configPath := filepath.Join(homeDir, configDir)

	godotenv.Load(filepath.Join(configPath, ".env"))
	godotenv.Load(".env")

	useGopass = os.Getenv("USE_GOPASS") == "true"

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

func chatWithAI(providerName, message string) {
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

	response, err := makeRequest(provider.Endpoint, provider.APIKey, req, provider.Name)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	if response.Error != nil {
		fmt.Printf("API Error: %s\n", response.Error.Message)
		return
	}

	if len(response.Choices) > 0 {
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
		chatWithAI(providerName, msg)
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
	fmt.Println("  terminal-ai web-server                  - Start web server")
	fmt.Println("  terminal-ai --help                       - Show this help")
	fmt.Println()
	fmt.Println("Providers (default: openrouter):")
	fmt.Println("  - openrouter")
	fmt.Println("  - gemini")
	fmt.Println("  - groq")
	fmt.Println()
	fmt.Println("Documentation:")
	fmt.Println("  docs/QUICKSTART.md     - Quick start guide")
	fmt.Println("  docs/SYSTEM.md         - Full system documentation")
	fmt.Println("  docs/SECURITY.md      - Security guide")
	fmt.Println("  docs/RASPBERRY_PI.md  - Raspberry Pi deployment")
}
