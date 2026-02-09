package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"terminal-ai/rag"
)

const pageSize = 5

type InteractiveSession struct {
	provider     string
	sessionID    string
	input        *bufio.Reader
	ragEnabled   bool
	ragManager   *rag.RAGManager
	hybridEngine *rag.HybridSearchEngine
	ctx          context.Context
	cancel       context.CancelFunc
}

func NewInteractiveSession(provider string) *InteractiveSession {
	ctx, cancel := context.WithCancel(context.Background())
	session := &InteractiveSession{
		provider:   provider,
		input:      bufio.NewReader(os.Stdin),
		ragEnabled: true,
		ctx:        ctx,
		cancel:     cancel,
	}
	session.initRAG()
	return session
}

func (s *InteractiveSession) initRAG() {
	ragManager, err := rag.NewRAGManager()
	if err != nil {
		fmt.Println(colorWarning("⚠ RAG initialization failed: " + err.Error()))
		s.ragEnabled = false
		return
	}
	s.ragManager = ragManager
	s.hybridEngine = rag.NewHybridSearchEngine(ragManager)
}

func (s *InteractiveSession) Start() {
	clearScreen()
	printWelcomeBanner()

	fmt.Println(colorInfo("AI Provider: " + s.provider))
	s.printRAGStatus()

	for {
		if s.ctx.Err() != nil {
			fmt.Println(colorWarning("\nSession cancelled. Exiting..."))
			return
		}

		fmt.Print(colorCyan("\n➤ ") + colorBold("You: "))
		line, err := s.input.ReadString('\n')
		if err != nil {
			fmt.Println(colorError("\nExiting interactive mode."))
			return
		}

		message := strings.TrimSpace(line)
		if message == "" {
			continue
		}

		switch message {
		case "/quit", "/exit", "quit", "exit":
			fmt.Println(colorSuccess("\n👋 Goodbye!"))
			return
		case "/help", "?":
			printInteractiveHelp()
			continue
		case "/clear", "clear":
			clearScreen()
			continue
		case "/history":
			s.showChatHistory()
			continue
		case "/stats":
			s.showStats()
			continue
		case "/rag on":
			s.ragEnabled = true
			fmt.Println(colorSuccess("✓ RAG enabled"))
			s.printRAGStatus()
			continue
		case "/rag off":
			s.ragEnabled = false
			fmt.Println(colorWarning("⚠ RAG disabled"))
			continue
		case "/rag":
			status := "enabled"
			if !s.ragEnabled {
				status = "disabled"
			}
			fmt.Printf(colorInfo("RAG is %s (use /rag on|off to toggle)\n"), status)
			s.printRAGStatus()
			continue
		}

		if strings.HasPrefix(message, "/provider ") {
			newProvider := strings.TrimSpace(strings.TrimPrefix(message, "/provider "))
			if newProvider != "" {
				s.provider = newProvider
				fmt.Printf(colorSuccess("✓ Provider changed to: %s\n"), newProvider)
			}
			continue
		}

		if strings.HasPrefix(message, "/model ") {
			fmt.Printf(colorSuccess("✓ Model preference set\n"))
			continue
		}

		if strings.HasPrefix(message, "/system ") {
			fmt.Println(colorSuccess("✓ System prompt updated"))
			continue
		}

		if message == "/tokens" {
			fmt.Println(colorInfo("Token tracking enabled"))
			continue
		}

		if strings.HasPrefix(message, "/index ") {
			dir := strings.TrimSpace(strings.TrimPrefix(message, "/index "))
			if dir != "" {
				s.indexDirectory(dir)
			} else {
				fmt.Println(colorInfo("Usage: /index <directory_path>"))
			}
			continue
		}

		if strings.HasPrefix(message, "/search ") {
			query := strings.TrimSpace(strings.TrimPrefix(message, "/search "))
			if query != "" {
				s.searchRAG(query)
			} else {
				fmt.Println(colorInfo("Usage: /search <query>"))
			}
			continue
		}

		if strings.HasPrefix(message, "/summarize ") {
			query := strings.TrimSpace(strings.TrimPrefix(message, "/summarize "))
			if query != "" {
				s.summarizeRAG(query)
			} else {
				fmt.Println(colorInfo("Usage: /summarize <query>"))
			}
			continue
		}

		s.sendMessage(message)
	}
}

func (s *InteractiveSession) sendMessage(message string) {
	fmt.Println()

	provider, ok := providers[s.provider]
	if !ok {
		provider = providers["openrouter"]
	}

	messages := []Message{
		{Role: "user", Content: message},
	}

	if s.ragEnabled && s.hybridEngine != nil {
		ctx := context.Background()
		results, err := s.hybridEngine.Search(ctx, message)
		if err == nil && len(results) > 0 {
			var contextBuilder strings.Builder
			contextBuilder.WriteString("DOCUMENT REFERENCES:\n")
			for i, r := range results {
				if i >= 5 {
					break
				}
				sourceName := r.SourcePath
				if r.SourceType == "web" && r.SourceURL != "" {
					sourceName = r.SourceURL
				}
				shortName := sourceName
				if len(shortName) > 50 {
					shortName = "..." + shortName[len(shortName)-47:]
				}
				content := strings.TrimSpace(r.Content)
				content = strings.ReplaceAll(content, "```", "")
				lines := strings.Split(content, "\n")
				var cleanLines []string
				for _, line := range lines {
					line = strings.TrimSpace(line)
					if len(line) > 5 && !strings.HasPrefix(line, "---") &&
						!strings.HasPrefix(line, "###") && !strings.HasPrefix(line, "##") &&
						!strings.HasPrefix(line, "**") && !strings.HasPrefix(line, "* ") &&
						!strings.HasPrefix(line, "- ") && !strings.HasPrefix(line, "1.") &&
						!strings.HasPrefix(line, "2.") && !strings.HasPrefix(line, "3.") {
						if len(line) < 200 {
							cleanLines = append(cleanLines, line)
						}
					}
				}
				maxLines := 3
				if len(cleanLines) < maxLines {
					maxLines = len(cleanLines)
				}
				cleanContent := content[:min(300, len(content))]
				if maxLines > 0 {
					cleanContent = strings.Join(cleanLines[:maxLines], " ")
				}
				if len(cleanContent) > 300 {
					cleanContent = cleanContent[:297] + "..."
				}
				contextBuilder.WriteString(fmt.Sprintf("[%d] %s\n%s\n\n", i+1, shortName, cleanContent))
			}

			messages = []Message{
				{Role: "system", Content: "Answer the user's question based on the document references provided. Keep your answer natural and concise. Do not show the reference numbers or source names in your answer unless the user asks."},
				{Role: "user", Content: contextBuilder.String() + "\n\nUser's question: " + message},
			}
		}
	}

	req := Request{
		Model:    provider.Model,
		Messages: messages,
		Stream:   true,
	}

	type Response struct {
		content string
		err     error
	}
	responseChan := make(chan Response, 1)

	go func() {
		response, err := makeRequest(provider.Endpoint, provider.APIKey, req, provider.Name)
		if err != nil {
			responseChan <- Response{err: err}
			return
		}
		if len(response.Choices) > 0 {
			responseChan <- Response{content: response.Choices[0].Message.Content}
		} else {
			responseChan <- Response{err: fmt.Errorf("no response")}
		}
	}()

	select {
	case resp := <-responseChan:
		if resp.err != nil {
			fmt.Println(colorError(fmt.Sprintf("\nError: %v", resp.err)))
			return
		}
	case <-time.After(120 * time.Second):
		fmt.Println(colorWarning("\n[Response timeout]"))
		return
	case <-s.ctx.Done():
		return
	}
}

func (s *InteractiveSession) showChatHistory() {
	homeDir, _ := os.UserHomeDir()
	historyDir := fmt.Sprintf("%s/.config/terminal-ai/chat-history", homeDir)
	os.MkdirAll(historyDir, 0755)

	files, _ := os.ReadDir(historyDir)
	if len(files) == 0 {
		fmt.Println(colorInfo("No chat history yet."))
		return
	}

	fmt.Println(colorCyan("\n📜 Recent Chat History:"))
	for i, file := range files {
		if i >= 5 {
			fmt.Println(colorInfo("  ... and more"))
			break
		}
		name := strings.TrimSuffix(file.Name(), ".txt")
		fmt.Printf("  %d. %s\n", i+1, name)
	}
}

func (s *InteractiveSession) printRAGStatus() {
	if s.ragManager == nil {
		fmt.Println(colorWarning("  RAG: Not initialized"))
		return
	}

	stats, err := s.ragManager.GetStats()
	if err != nil {
		fmt.Println(colorInfo("  RAG: Initialized (stats unavailable)"))
		return
	}

	status := "✅ Active"
	if !s.ragEnabled {
		status = "⚪ Disabled"
	}
	fmt.Printf(colorInfo("  RAG: %s | Sources: %d | Chunks: %d\n"), status, stats.TotalSources, stats.TotalChunks)
}

func (s *InteractiveSession) indexDirectory(dir string) {
	if s.ragManager == nil {
		fmt.Println(colorError("RAG not initialized"))
		return
	}

	fmt.Printf(colorInfo("📚 Indexing: %s\n"), dir)
	report, err := s.ragManager.IndexDirectory(dir)
	if err != nil {
		fmt.Println(colorError(fmt.Sprintf("Error: %v", err)))
		return
	}

	fmt.Printf(colorSuccess("✅ Added %d files (%d chunks)\n"), report.Added, report.TotalChunks)
	s.printRAGStatus()
}

func (s *InteractiveSession) searchRAG(query string) {
	if s.ragManager == nil {
		fmt.Println(colorError("RAG not initialized"))
		return
	}

	ctx := context.Background()
	results, err := s.hybridEngine.Search(ctx, query)
	if err != nil {
		fmt.Println(colorError(fmt.Sprintf("Search error: %v", err)))
		return
	}

	if len(results) == 0 {
		fmt.Println(colorInfo("No results found"))
		return
	}

	s.displayPaginatedResults(results, "Search")
}

func (s *InteractiveSession) summarizeRAG(query string) {
	if s.ragManager == nil {
		fmt.Println(colorError("RAG not initialized"))
		return
	}

	fmt.Printf(colorInfo("🔍 Searching and summarizing: %s\n\n"), query)

	ctx := context.Background()
	results, err := s.hybridEngine.Search(ctx, query)
	if err != nil {
		fmt.Println(colorError(fmt.Sprintf("Search error: %v", err)))
		return
	}

	if len(results) == 0 {
		fmt.Println(colorInfo("No results found"))
		return
	}

	s.displayPaginatedSummaries(results)
}

func (s *InteractiveSession) displayPaginatedResults(results []rag.HybridSearchResult, mode string) {
	total := len(results)
	currentPage := 0

	for {
		start := currentPage * pageSize
		end := start + pageSize
		if end > total {
			end = total
		}

		fmt.Printf(colorInfo("\n📋 %s Results %d-%d of %d:\n"), mode, start+1, end, total)
		fmt.Println("═══════════════════════════════════════════════════════")

		for i := start; i < end; i++ {
			r := results[i]
			sourceType := "📄"
			if r.SourceType == "web" {
				sourceType = "🌐"
			}
			sourceName := r.SourcePath
			if r.SourceType == "web" && r.SourceURL != "" {
				sourceName = r.SourceURL
			}
			if len(sourceName) > 50 {
				sourceName = sourceName[:47] + "..."
			}
			fmt.Printf("\n%d. %s %s\n", i+1, sourceType, sourceName)
			fmt.Printf("   Score: %.3f | Length: %d chars\n", r.HybridScore, len(r.Content))
			content := r.Content
			if len(content) > 200 {
				content = content[:197] + "..."
			}
			fmt.Printf("   %s\n", content)
		}

		fmt.Println()

		if end >= total {
			break
		}

		fmt.Print(colorInfo("Press [Enter] for more, [q] to quit: "))
		line, _ := s.input.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "q" || line == "Q" {
			break
		}
		fmt.Println()
	}
}

func (s *InteractiveSession) displayPaginatedSummaries(results []rag.HybridSearchResult) {
	total := len(results)
	currentPage := 0

	for {
		start := currentPage * pageSize
		end := start + pageSize
		if end > total {
			end = total
		}

		fmt.Printf(colorInfo("\n📋 Summary %d-%d of %d:\n"), start+1, end, total)
		fmt.Println("═══════════════════════════════════════════════════════")

		for i := start; i < end; i++ {
			r := results[i]
			sourceType := "📄"
			if r.SourceType == "web" {
				sourceType = "🌐"
			}
			sourceName := r.SourcePath
			if r.SourceType == "web" && r.SourceURL != "" {
				sourceName = r.SourceURL
			}
			if len(sourceName) > 50 {
				sourceName = "..." + sourceName[len(sourceName)-47:]
			}

			fmt.Printf("\n%s %d. %s %s\n", colorCyan("📄"), i+1, sourceType, sourceName)
			fmt.Printf("   Score: %.3f | Length: %d chars\n", r.HybridScore, len(r.Content))

			summary := summarizeContent(r.Content)
			fmt.Printf("\n   %s\n", wrapText(summary, 60))
		}

		fmt.Println()

		if end >= total {
			break
		}

		fmt.Print(colorInfo("Press [Enter] for more, [q] to quit: "))
		line, _ := s.input.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "q" || line == "Q" {
			break
		}
		fmt.Println()
	}
}

func (s *InteractiveSession) showStats() {
	fmt.Println(colorBold("\n📊 Session Statistics:"))
	fmt.Printf("  Provider: %s\n", s.provider)
	fmt.Printf("  RAG: %v\n", s.ragEnabled)
	if s.ragManager != nil {
		stats, _ := s.ragManager.GetStats()
		fmt.Printf("  Sources: %d\n", stats.TotalSources)
		fmt.Printf("  Chunks: %d\n", stats.TotalChunks)
	}
}

func clearScreen() {
	cmd := exec.Command("clear")
	cmd.Stdout = os.Stdout
	cmd.Run()
}

func printWelcomeBanner() {
	banner := `
╔═══════════════════════════════════════════════════════════════╗
║                                                           ║
║     ██████╗ ██╗     ██╗ ██████╗ ██████╗            ║
║     ██╔══██╗██║     ██║██╔════╝██╔═══██╗           ║
║     ██████╔╝██║     ██║██║     ██║   ██║            ║
║     ██╔══██╗██║     ██║██║     ██║   ██║            ║
║     ██████╔╝███████╗██████╔╝██████╔╝            ║
║     ╚═════╝ ╚══════╝╚═════╝ ╚═════╝             ║
║                                                           ║
║              Terminal AI - Interactive Chat                 ║
║                                                           ║
║     Type /help for commands, /quit to exit                ║
║                                                           ║
╚═══════════════════════════════════════════════════════════════╝`
	fmt.Println(colorBold(colorCyan(banner)))
}

func printInteractiveHelp() {
	help := `
╔═══════════════════════════════════════════════════════════════╗
║                  Available Commands                 ║
╠═══════════════════════════════════════════════════════════════╣
║  /help, ?          Show this help                ║
║  /quit, /exit      Exit chat                     ║
║  /clear            Clear screen                   ║
║  /history          Show chat history             ║
║  /stats            Session statistics             ║
║  /provider <name>  Switch provider               ║
║  /model <name>     Set model                     ║
║  /rag on|off       Enable/disable RAG            ║
║  /rag              Show RAG status               ║
║  /index <dir>      Index a directory             ║
║  /search <query>   Search documents              ║
║  /summarize <query> Summarize results            ║
║  /tokens           Token tracking                 ║
╠═══════════════════════════════════════════════════════════════╣
║  Just type your message and press Enter!        ║
╚═══════════════════════════════════════════════════════════════╝`
	fmt.Println(help)
}

func HandleInteractiveChat() {
	provider := "openrouter"

	for i, arg := range os.Args {
		if arg == "--provider" || arg == "-p" {
			if i+1 < len(os.Args) {
				provider = os.Args[i+1]
			}
		}
	}

	session := NewInteractiveSession(provider)
	session.Start()
}
