package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type TelegramBot struct {
	api          *tgbotapi.BotAPI
	token        string
	webhookURL   string
	allowedUsers map[int64]bool
	sessionStore map[int64]*BotSession
	mu           sync.RWMutex
	webhookMode  bool
}

type BotSession struct {
	UserID      int64
	ChatID      int64
	Mode        string
	Provider    string
	Messages    []Message
	LastActive  time.Time
	Interactive bool
}

var telegramBot *TelegramBot

const (
	MODE_CHAT        = "chat"
	MODE_INTERACTIVE = "interactive"
)

func initTelegramBot() error {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		return fmt.Errorf("TELEGRAM_BOT_TOKEN not set")
	}

	webhookURL := os.Getenv("TELEGRAM_WEBHOOK_URL")
	allowedUsersStr := os.Getenv("TELEGRAM_ALLOWED_USERS")

	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return fmt.Errorf("failed to create bot: %w", err)
	}

	allowedUsers := make(map[int64]bool)
	if allowedUsersStr != "" {
		users := strings.Split(allowedUsersStr, ",")
		for _, u := range users {
			u = strings.TrimSpace(u)
			if u == "" {
				continue
			}
			userID, err := strconv.ParseInt(u, 10, 64)
			if err != nil {
				continue
			}
			allowedUsers[userID] = true
		}
	}

	bot := &TelegramBot{
		api:          api,
		token:        token,
		webhookURL:   webhookURL,
		allowedUsers: allowedUsers,
		sessionStore: make(map[int64]*BotSession),
		webhookMode:  webhookURL != "",
	}

	if webhookURL != "" {
		if err := bot.setupWebhook(); err != nil {
			return fmt.Errorf("failed to setup webhook: %w", err)
		}
	}

	telegramBot = bot
	return nil
}

func (b *TelegramBot) setupWebhook() error {
	whURL := fmt.Sprintf("%s/telegram/webhook", b.webhookURL)
	wh, _ := tgbotapi.NewWebhook(whURL)
	_, err := b.api.Request(wh)
	if err != nil {
		return err
	}

	webhookInfo, err := b.api.GetWebhookInfo()
	if err != nil {
		return err
	}

	fmt.Printf("Telegram webhook set to: %s\n", webhookInfo.URL)
	return nil
}

func (b *TelegramBot) isAllowed(userID int64) bool {
	if len(b.allowedUsers) == 0 {
		return true
	}
	return b.allowedUsers[userID]
}

func (b *TelegramBot) getSession(chatID int64) *BotSession {
	b.mu.Lock()
	defer b.mu.Unlock()

	session, exists := b.sessionStore[chatID]
	if !exists {
		session = &BotSession{
			ChatID:     chatID,
			Mode:       MODE_CHAT,
			Provider:   providerConfig.DefaultProvider,
			Messages:   []Message{},
			LastActive: time.Now(),
		}
		b.sessionStore[chatID] = session
	} else {
		session.LastActive = time.Now()
	}
	return session
}

func (b *TelegramBot) handleUpdate(update tgbotapi.Update) {
	if update.Message == nil {
		return
	}

	msg := update.Message
	chatID := msg.Chat.ID
	userID := msg.From.ID

	if !b.isAllowed(userID) {
		b.sendMessage(chatID, "Anda tidak dibenarkan menggunakan bot ini.")
		return
	}

	text := strings.TrimSpace(msg.Text)

	if text == "" {
		return
	}

	if text == "/start" || text == "/help" {
		b.sendHelp(chatID)
		return
	}

	session := b.getSession(chatID)

	if strings.HasPrefix(text, "/") {
		b.handleCommand(chatID, text, session, userID)
	} else {
		b.handleMessage(chatID, text, session)
	}
}

func (b *TelegramBot) handleCommand(chatID int64, text string, session *BotSession, userID int64) {
	parts := strings.Fields(text)
	cmd := strings.ToLower(parts[0])
	args := ""
	if len(parts) > 1 {
		args = strings.TrimSpace(strings.Join(parts[1:], " "))
	}

	switch cmd {
	case "/help", "/start":
		b.sendHelp(chatID)

	case "/provider":
		b.handleProviderCommand(chatID, args, session)

	case "/model":
		b.handleModelCommand(chatID, args, session)

	case "/rag":
		b.handleRAGCommand(chatID, args)

	case "/interactive":
		b.startInteractiveMode(chatID, session)

	case "/quit", "/exit":
		b.stopInteractiveMode(chatID, session)

	case "/clear":
		b.clearSession(chatID, session)
		b.sendMessage(chatID, "Session cleared")

	case "/history":
		b.showHistory(chatID, session)

	case "/memory":
		b.handleMemoryCommand(chatID, args)

	case "/web":
		b.handleWebCommand(chatID, args)

	case "/status":
		b.showStatus(chatID, session)

	case "/session":
		b.handleSessionCommand(chatID, args, session)

	case "/chat":
		if args == "" {
			b.sendMessage(chatID, "Usage: /chat <message>\nContoh: /chat Apa itu quantum computing?")
			return
		}
		b.handleAICommand(chatID, args, session)

	case "/userinfo":
		b.handleUserInfoCommand(chatID, args, session)

	case "/allow":
		b.handleAllowUserCommand(chatID, args, session)

	case "/block":
		b.handleBlockUserCommand(chatID, args, session)

	case "/userlist":
		b.handleUserListCommand(chatID, session)

	default:
		if strings.HasPrefix(text, "/") {
			b.handleAICommand(chatID, text, session)
		} else {
			b.handleMessage(chatID, text, session)
		}
	}
}

func (b *TelegramBot) handleProviderCommand(chatID int64, args string, session *BotSession) {
	args = strings.TrimSpace(args)
	parts := strings.Fields(args)

	if len(parts) == 0 {
		b.sendMessage(chatID, b.formatProviderList())
		return
	}

	subCmd := strings.ToLower(parts[0])

	switch subCmd {
	case "list":
		b.sendMessage(chatID, b.formatProviderList())

	case "current":
		b.sendMessage(chatID, fmt.Sprintf("Provider semasa: %s", session.Provider))

	case "set":
		if len(parts) < 2 {
			b.sendMessage(chatID, "Usage: /provider set <nama-provider>\nContoh: /provider set groq")
			return
		}
		providerName := strings.ToLower(parts[1])
		if _, exists := providers[providerName]; !exists {
			b.sendMessage(chatID, fmt.Sprintf("Provider '%s' tidak wujud.\nGuna /provider list untuk senarai provider.", providerName))
			return
		}
		session.Provider = providerName
		b.sendMessage(chatID, fmt.Sprintf("Provider ditukar ke: %s", providerName))

	default:
		b.sendMessage(chatID, "Usage: /provider list | /provider current | /provider set <nama>")
	}
}

func (b *TelegramBot) formatProviderList() string {
	var sb strings.Builder
	sb.WriteString("Provider Configuration:\n\n")

	orderedProviders := getOrderedProviders()

	for i, providerName := range orderedProviders {
		config := providerConfig.Providers[providerName]
		provider := providers[providerName]

		status := "Enabled"
		if !config.Enabled {
			status = "Disabled"
		}

		defaultMarker := ""
		if providerName == providerConfig.DefaultProvider {
			defaultMarker = " (DEFAULT)"
		}

		sb.WriteString(fmt.Sprintf("%d. %s%s [%s]\n", i+1, providerName, defaultMarker, status))
		sb.WriteString(fmt.Sprintf("   Priority: %d\n", config.Priority))
		sb.WriteString(fmt.Sprintf("   Model: %s\n\n", provider.Model))
	}

	return sb.String()
}

func (b *TelegramBot) handleModelCommand(chatID int64, args string, session *BotSession) {
	args = strings.TrimSpace(args)
	parts := strings.Fields(args)

	if len(parts) == 0 {
		provider := providers[session.Provider]
		b.sendMessage(chatID, fmt.Sprintf("Model semasa: %s\nGuna /model <nama-model> untuk tukar", provider.Model))
		return
	}

	b.sendMessage(chatID, "Model selection akan datang kelak.")
}

func (b *TelegramBot) handleRAGCommand(chatID int64, args string) {
	args = strings.TrimSpace(args)
	parts := strings.Fields(args)

	if len(parts) == 0 {
		b.sendRAGStatus(chatID)
		return
	}

	subCmd := strings.ToLower(parts[0])

	switch subCmd {
	case "on":
		b.sendMessage(chatID, "RAG enabled.")

	case "off":
		b.sendMessage(chatID, "RAG disabled.")

	case "status":
		b.sendRAGStatus(chatID)

	case "search":
		if len(parts) < 2 {
			b.sendMessage(chatID, "Usage: /rag search <query>\nContoh: /rag search docker")
			return
		}
		query := strings.TrimSpace(strings.Join(parts[1:], " "))
		b.searchRAGAndRespond(chatID, query)

	case "index":
		if len(parts) < 2 {
			b.sendMessage(chatID, "Usage: /rag index <directory>\nContoh: /rag index ~/documents")
			return
		}
		dir := strings.TrimSpace(strings.Join(parts[1:], " "))
		b.indexDirectory(chatID, dir)

	case "list":
		b.listRAGSources(chatID)

	default:
		b.sendRAGUsage(chatID)
	}
}

func (b *TelegramBot) sendRAGStatus(chatID int64) {
	dataDir := getDataDir()
	indexFile := fmt.Sprintf("%s/rag-index.json", dataDir)

	var count int
	if _, err := os.Stat(indexFile); err == nil {
		data, _ := os.ReadFile(indexFile)
		var idx RAGIndex
		json.Unmarshal(data, &idx)
		count = len(idx.Documents)
	}

	ragStatus := "Inactive"
	if _, err := os.Stat(indexFile); err == nil {
		ragStatus = "Active"
	}

	b.sendMessage(chatID, fmt.Sprintf("RAG Status:\n\nStatus: %s\nDocuments indexed: %d\n\nCommands:\n/rag on - Enable RAG\n/rag off - Disable RAG\n/rag search <query> - Search documents\n/rag index <dir> - Index directory\n/rag status - Show RAG status", ragStatus, count))
}

func (b *TelegramBot) sendRAGUsage(chatID int64) {
	b.sendMessage(chatID, "RAG Commands:\n\n/rag on - Enable RAG context\n/rag off - Disable RAG context\n/rag search <query> - Search indexed documents\n/rag index <dir> - Index a directory\n/rag status - Show RAG status\n/rag list - List indexed sources")
}

func (b *TelegramBot) searchRAGAndRespond(chatID int64, query string) {
	results := searchRAG(query)

	if len(results) == 0 {
		b.sendMessage(chatID, fmt.Sprintf("Tiada results untuk: '%s'", query))
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d result(s) untuk '%s':\n\n", len(results), query))

	for i, doc := range results {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, doc.Path))
		preview := doc.Content
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		sb.WriteString(fmt.Sprintf("   %s\n\n", preview))
	}

	b.sendMessage(chatID, sb.String())
}

func (b *TelegramBot) indexDirectory(chatID int64, dir string) {
	b.sendMessage(chatID, fmt.Sprintf("Indexing directory: %s...", dir))

	go func() {
		indexDirectory(dir)
		if telegramBot != nil {
			telegramBot.sendMessage(chatID, fmt.Sprintf("Done indexing: %s", dir))
		}
	}()
}

func (b *TelegramBot) listRAGSources(chatID int64) {
	dataDir := getDataDir()
	indexFile := fmt.Sprintf("%s/rag-index.json", dataDir)

	if _, err := os.Stat(indexFile); os.IsNotExist(err) {
		b.sendMessage(chatID, "Tiada sources di-index.")
		return
	}

	data, _ := os.ReadFile(indexFile)
	var idx RAGIndex

	if err := json.Unmarshal(data, &idx); err != nil {
		b.sendMessage(chatID, "Error reading index.")
		return
	}

	if len(idx.Documents) == 0 {
		b.sendMessage(chatID, "Tiada sources di-index.")
		return
	}

	var sb strings.Builder
	sb.WriteString("Indexed Sources:\n\n")

	paths := make(map[string]int)
	for _, doc := range idx.Documents {
		paths[doc.Path]++
	}

	for path, count := range paths {
		sb.WriteString(fmt.Sprintf("%s (%d files)\n", path, count))
	}

	b.sendMessage(chatID, sb.String())
}

func (b *TelegramBot) startInteractiveMode(chatID int64, session *BotSession) {
	session.Interactive = true
	session.Mode = MODE_INTERACTIVE
	session.Messages = []Message{}

	b.sendMessage(chatID, "Entering Interactive Mode\n\n"+"Dalam mode ini:\n"+"Taip mesej untuk chat dengan AI\n"+" /help - Lihat commands\n"+" /quit - Keluar dari interactive mode\n"+" /rag on/off - Toggle RAG context\n\n"+"Anda: ")
}

func (b *TelegramBot) stopInteractiveMode(chatID int64, session *BotSession) {
	session.Interactive = false
	session.Mode = MODE_CHAT
	session.Messages = []Message{}

	b.sendMessage(chatID, "Exited interactive mode.")
}

func (b *TelegramBot) clearSession(chatID int64, session *BotSession) {
	session.Messages = []Message{}
}

func (b *TelegramBot) showHistory(chatID int64, session *BotSession) {
	if len(session.Messages) == 0 {
		b.sendMessage(chatID, "Tiada history dalam session ini.")
		return
	}

	var sb strings.Builder
	sb.WriteString("Chat History:\n\n")

	maxMsgs := 10
	if len(session.Messages) < maxMsgs {
		maxMsgs = len(session.Messages)
	}

	for i := len(session.Messages) - maxMsgs; i < len(session.Messages); i++ {
		msg := session.Messages[i]
		role := "User"
		if msg.Role == "assistant" {
			role = "AI"
		}
		preview := msg.Content
		if len(preview) > 100 {
			preview = preview[:100] + "..."
		}
		sb.WriteString(fmt.Sprintf("%s: %s\n\n", role, preview))
	}

	b.sendMessage(chatID, sb.String())
}

func (b *TelegramBot) handleMemoryCommand(chatID int64, args string) {
	args = strings.TrimSpace(args)
	parts := strings.Fields(args)

	if len(parts) == 0 {
		b.sendMemoryHelp(chatID)
		return
	}

	subCmd := strings.ToLower(parts[0])

	switch subCmd {
	case "add":
		if len(parts) < 2 {
			b.sendMessage(chatID, "Usage: /memory add <text>\nContoh: /memory add Nama saya Ahmad")
			return
		}
		text := strings.TrimSpace(strings.Join(parts[1:], " "))
		b.addMemory(chatID, text)

	case "recall", "search":
		if len(parts) < 2 {
			b.sendMessage(chatID, "Usage: /memory recall <query>\nContoh: /memory recall nama saya")
			return
		}
		query := strings.TrimSpace(strings.Join(parts[1:], " "))
		b.recallMemory(chatID, query)

	case "list":
		b.listMemories(chatID)

	default:
		b.sendMemoryHelp(chatID)
	}
}

func (b *TelegramBot) sendMemoryHelp(chatID int64) {
	b.sendMessage(chatID, "Memory Commands:\n\n/memory add <text> - Tambah memory\n/memory recall <query> - Cari memory\n/memory list - Senarai memories\n\nMemory menggunakan vector database untuk semantic search.")
}

func (b *TelegramBot) addMemory(chatID int64, text string) {
	b.sendMessage(chatID, "Adding memory...")

	go func() {
		ctx := context.Background()
		mgr := GetMemoryManager()
		if mgr == nil {
			if telegramBot != nil {
				telegramBot.sendMessage(chatID, "Memory manager not initialized.")
			}
			return
		}

		metadata := MemoryMetadata{
			Source: "telegram",
			Tags:   []string{"telegram"},
		}

		_, err := mgr.AddMemory(ctx, text, metadata)
		if telegramBot != nil {
			if err != nil {
				telegramBot.sendMessage(chatID, fmt.Sprintf("Error adding memory: %v", err))
			} else {
				telegramBot.sendMessage(chatID, "Memory added successfully.")
			}
		}
	}()
}

func (b *TelegramBot) recallMemory(chatID int64, query string) {
	b.sendMessage(chatID, "Searching memories...")

	go func() {
		ctx := context.Background()
		mgr := GetMemoryManager()
		if mgr == nil {
			if telegramBot != nil {
				telegramBot.sendMessage(chatID, "Memory manager not initialized.")
			}
			return
		}

		results, err := mgr.SearchMemories(ctx, query, 5)
		if telegramBot != nil {
			if err != nil {
				telegramBot.sendMessage(chatID, fmt.Sprintf("Error searching memories: %v", err))
				return
			}

			if len(results) == 0 {
				telegramBot.sendMessage(chatID, "Tiada memory found.")
				return
			}

			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Found %d memory(ies):\n\n", len(results)))

			for i, result := range results {
				preview := result.Memory.Content
				if len(preview) > 150 {
					preview = preview[:150] + "..."
				}
				sb.WriteString(fmt.Sprintf("%d. %s\n\n", i+1, preview))
			}

			telegramBot.sendMessage(chatID, sb.String())
		}
	}()
}

func (b *TelegramBot) listMemories(chatID int64) {
	b.sendMessage(chatID, "Listing memories... (coming soon)")
}

func (b *TelegramBot) handleWebCommand(chatID int64, url string) {
	url = strings.TrimSpace(url)

	if url == "" || !strings.HasPrefix(url, "http") {
		b.sendMessage(chatID, "Usage: /web <url>\nContoh: /web https://example.com")
		return
	}

	b.sendMessage(chatID, fmt.Sprintf("Fetching: %s...", url))

	go func() {
		content := fetchWebPage(url)
		if telegramBot != nil {
			if len(content) > 1000 {
				content = content[:1000] + "...\n\n(snip)"
			}
			telegramBot.sendMessage(chatID, fmt.Sprintf("Content:\n\n%s", content))
		}
	}()
}

func fetchWebPage(url string) string {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Sprintf("Error reading: %v", err)
	}

	content := string(body)
	content = removeHTMLTags(content)

	return content
}

func removeHTMLTags(html string) string {
	re := regexp.MustCompile(`<[^>]*>`)
	return re.ReplaceAllString(html, "")
}

func (b *TelegramBot) showStatus(chatID int64, session *BotSession) {
	provider := providers[session.Provider]

	var sb strings.Builder
	sb.WriteString("Status:\n\n")
	sb.WriteString(fmt.Sprintf("Mode: %s\n", session.Mode))
	sb.WriteString(fmt.Sprintf("Provider: %s\n", session.Provider))
	sb.WriteString(fmt.Sprintf("Model: %s\n", provider.Model))
	sb.WriteString(fmt.Sprintf("Messages: %d\n", len(session.Messages)))

	dataDir := getDataDir()
	indexFile := fmt.Sprintf("%s/rag-index.json", dataDir)
	ragStatus := "Disabled"
	if _, err := os.Stat(indexFile); err == nil {
		ragStatus = "Enabled"
	}
	sb.WriteString(fmt.Sprintf("RAG: %s\n", ragStatus))

	b.sendMessage(chatID, sb.String())
}

func (b *TelegramBot) handleSessionCommand(chatID int64, args string, session *BotSession) {
	args = strings.TrimSpace(args)

	if args == "" || args == "info" {
		b.showStatus(chatID, session)
		return
	}

	switch strings.ToLower(args) {
	case "new":
		b.clearSession(chatID, session)
		b.sendMessage(chatID, "New session started.")

	default:
		b.sendMessage(chatID, "Usage: /session info | /session new")
	}
}

func (b *TelegramBot) handleMessage(chatID int64, text string, session *BotSession) {
	if session.Interactive {
		b.handleInteractiveMessage(chatID, text, session)
	} else {
		b.handleAICommand(chatID, text, session)
	}
}

func (b *TelegramBot) handleInteractiveMessage(chatID int64, text string, session *BotSession) {
	if text == "/quit" || text == "/exit" {
		b.stopInteractiveMode(chatID, session)
		return
	}

	if text == "/help" {
		b.sendMessage(chatID, "Interactive Mode Commands:\n\n/quit - Keluar\n/rag on/off - Toggle RAG\n/history - Lihat history\n/clear - Clear history")
		return
	}

	session.Messages = append(session.Messages, Message{Role: "user", Content: text})

	b.sendMessage(chatID, "AI is thinking...")

	go func() {
		provider := providers[session.Provider]

		var messages []Message
		messages = append(messages, session.Messages...)

		if len(session.Messages) > 10 {
			messages = session.Messages[len(session.Messages)-10:]
		}

		response, err := makeRequest(provider.Endpoint, provider.APIKey, Request{
			Model:    provider.Model,
			Messages: messages,
			Stream:   false,
		}, provider.Name)

		if telegramBot == nil {
			return
		}

		if err != nil {
			telegramBot.sendMessage(chatID, fmt.Sprintf("Error: %v", err))
			return
		}

		if response.Error != nil {
			telegramBot.sendMessage(chatID, fmt.Sprintf("API Error: %s", response.Error.Message))
			return
		}

		if len(response.Choices) > 0 {
			content := response.Choices[0].Message.Content
			session.Messages = append(session.Messages, Message{Role: "assistant", Content: content})

			if len(content) > 4000 {
				for i := 0; i < len(content); i += 4000 {
					end := i + 4000
					if end > len(content) {
						end = len(content)
					}
					part := content[i:end]
					if i+4000 < len(content) {
						part += "... (continues)"
					}
					telegramBot.sendMessage(chatID, part)
					time.Sleep(500 * time.Millisecond)
				}
			} else {
				telegramBot.sendMessage(chatID, content)
			}

			telegramBot.sendMessage(chatID, "\nAnda: ")
		} else {
			telegramBot.sendMessage(chatID, "Tiada response received.")
		}
	}()
}

func (b *TelegramBot) handleAICommand(chatID int64, text string, session *BotSession) {
	text = strings.TrimSpace(text)

	if text == "" {
		return
	}

	if strings.HasPrefix(text, "/chat") {
		text = strings.TrimPrefix(text, "/chat")
		text = strings.TrimSpace(text)
		if text == "" {
			b.sendMessage(chatID, "Usage: /chat <message>\nContoh: /chat Apa itu quantum computing?")
			return
		}
	} else if strings.HasPrefix(text, "/") && !strings.HasPrefix(text, "/ ") {
		b.sendMessage(chatID, "Command tidak dikenali. Taip /help untuk bantuan.")
		return
	}

	b.sendMessage(chatID, "Processing...")

	provider := providers[session.Provider]

	messages := []Message{{Role: "user", Content: text}}

	response, err := makeRequest(provider.Endpoint, provider.APIKey, Request{
		Model:    provider.Model,
		Messages: messages,
		Stream:   false,
	}, provider.Name)

	if err != nil {
		b.sendMessage(chatID, fmt.Sprintf("Error: %v", err))
		return
	}

	if response.Error != nil {
		b.sendMessage(chatID, fmt.Sprintf("API Error: %s", response.Error.Message))
		return
	}

	if len(response.Choices) > 0 {
		content := response.Choices[0].Message.Content

		if len(content) > 4000 {
			for i := 0; i < len(content); i += 4000 {
				end := i + 4000
				if end > len(content) {
					end = len(content)
				}
				part := content[i:end]
				if i+4000 < len(content) {
					part += "... (continues)"
				}
				b.sendMessage(chatID, part)
				time.Sleep(500 * time.Millisecond)
			}
		} else {
			b.sendMessage(chatID, content)
		}
	} else {
		b.sendMessage(chatID, "Tiada response received.")
	}
}

func (b *TelegramBot) sendHelp(chatID int64) {
	helpText := `Terminal AI Bot

Commands:
/help - Show this message
/chat <message> - Chat dengan AI
/interactive - Enter interactive mode
/quit - Exit interactive mode

Provider Commands:
/provider list - Senarai providers
/provider current - Provider semasa
/provider set <nama> - Tukar provider

RAG Commands:
/rag on/off - Enable/disable RAG
/rag search <query> - Search documents
/rag index <dir> - Index directory
/rag status - Show RAG status

Memory Commands:
/memory add <text> - Tambah memory
/memory recall <query> - Cari memory
/memory list - Senarai memories

Utility:
/web <url> - Fetch web page
/history - Chat history
/clear - Clear session
/status - Show status
/session info - Session info

Admin Commands:
/userinfo <id> - Maklumat user
/allow <id> - Benarkan user
/block <id> - Sekat user
/userlist - Senarai users

Examples:
/chat Apa itu quantum computing?
/rag search docker
/provider set groq
/allow 123456789
`

	b.sendMessage(chatID, helpText)
}

func (b *TelegramBot) sendMessage(chatID int64, text string) {
	if len(text) > 4096 {
		text = text[:4096]
	}

	msg := tgbotapi.NewMessage(chatID, text)
	b.api.Send(msg)
}

func (b *TelegramBot) sendTyping(chatID int64) {
	typing := tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping)
	b.api.Send(typing)
}

func startTelegramLongPolling() {
	if telegramBot == nil {
		return
	}

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := telegramBot.api.GetUpdatesChan(u)

	for update := range updates {
		telegramBot.handleUpdate(update)
	}
}

func handleTelegramWebhook(w http.ResponseWriter, r *http.Request) {
	if telegramBot == nil {
		http.Error(w, "Bot not initialized", http.StatusInternalServerError)
		return
	}

	var update tgbotapi.Update
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	go telegramBot.handleUpdate(update)

	w.WriteHeader(http.StatusOK)
}

func executeTelegramCommand(chatID int64, command string) string {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return "Empty command"
	}

	cmd := exec.Command(parts[0], parts[1:]...)
	output, err := cmd.Output()
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	return string(output)
}

func startTelegramBot() error {
	if telegramBot == nil {
		return nil
	}

	if telegramBot.webhookMode {
		return nil
	}

	go startTelegramLongPolling()
	return nil
}

type TelegramUser struct {
	UserID    int64  `json:"user_id"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Blocked   bool   `json:"blocked"`
	AddedAt   string `json:"added_at"`
}

var allowedUsersStore map[int64]*TelegramUser = make(map[int64]*TelegramUser)

func (b *TelegramBot) handleUserInfoCommand(chatID int64, args string, session *BotSession) {
	if !b.isAdmin(chatID) {
		b.sendMessage(chatID, "Anda tidak mempunyai kebenaran untuk menggunakan command ini.")
		return
	}

	args = strings.TrimSpace(args)
	parts := strings.Fields(args)

	if len(parts) == 0 {
		b.sendMessage(chatID, "Usage: /userinfo <user_id>\nContoh: /userinfo 123456789")
		return
	}

	targetUserID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		b.sendMessage(chatID, "Invalid user ID.")
		return
	}

	user, exists := allowedUsersStore[targetUserID]
	if !exists {
		b.sendMessage(chatID, fmt.Sprintf("User ID %d tidak dijumpai dalam sistem.", targetUserID))
		return
	}

	status := "Allowed"
	if user.Blocked {
		status = "Blocked"
	}

	username := "@" + user.Username
	if user.Username == "" {
		username = "(tiada)"
	}

	b.sendMessage(chatID, fmt.Sprintf("📋 User Info:\n\n🆔 ID: %d\n👤 Username: %s\n📛 First Name: %s\n📛 Last Name: %s\n📊 Status: %s\n📅 Added: %s",
		user.UserID, username, user.FirstName, user.LastName, status, user.AddedAt))
}

func (b *TelegramBot) handleAllowUserCommand(chatID int64, args string, session *BotSession) {
	if !b.isAdmin(chatID) {
		b.sendMessage(chatID, "Anda tidak mempunyai kebenaran untuk menggunakan command ini.")
		return
	}

	args = strings.TrimSpace(args)
	parts := strings.Fields(args)

	if len(parts) == 0 {
		b.sendMessage(chatID, "Usage: /allow <user_id>\nContoh: /allow 123456789")
		return
	}

	targetUserID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		b.sendMessage(chatID, "Invalid user ID. Sila masukkan nombor.")
		return
	}

	var username string
	if len(parts) > 1 {
		username = parts[1]
	} else {
		username = "unknown"
	}

	if _, exists := allowedUsersStore[targetUserID]; exists {
		allowedUsersStore[targetUserID].Blocked = false
		b.sendMessage(chatID, fmt.Sprintf("✅ User ID %d telah dibenarkan semula.", targetUserID))
	} else {
		allowedUsersStore[targetUserID] = &TelegramUser{
			UserID:    targetUserID,
			Username:  username,
			FirstName: "",
			LastName:  "",
			Blocked:   false,
			AddedAt:   time.Now().Format("2006-01-02 15:04"),
		}
		b.sendMessage(chatID, fmt.Sprintf("✅ User ID %d telah dibenarkan.", targetUserID))
	}

	saveAllowedUsers()
}

func (b *TelegramBot) handleBlockUserCommand(chatID int64, args string, session *BotSession) {
	if !b.isAdmin(chatID) {
		b.sendMessage(chatID, "Anda tidak mempunyai kebenaran untuk menggunakan command ini.")
		return
	}

	args = strings.TrimSpace(args)
	parts := strings.Fields(args)

	if len(parts) == 0 {
		b.sendMessage(chatID, "Usage: /block <user_id>\nContoh: /block 123456789")
		return
	}

	targetUserID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		b.sendMessage(chatID, "Invalid user ID. Sila masukkan nombor.")
		return
	}

	if _, exists := allowedUsersStore[targetUserID]; !exists {
		allowedUsersStore[targetUserID] = &TelegramUser{
			UserID:    targetUserID,
			Username:  "unknown",
			FirstName: "",
			LastName:  "",
			Blocked:   true,
			AddedAt:   time.Now().Format("2006-01-02 15:04"),
		}
	} else {
		allowedUsersStore[targetUserID].Blocked = true
	}

	b.sendMessage(chatID, fmt.Sprintf("❌ User ID %d telah disekat.", targetUserID))
	saveAllowedUsers()
}

func (b *TelegramBot) handleUserListCommand(chatID int64, session *BotSession) {
	if !b.isAdmin(chatID) {
		b.sendMessage(chatID, "Anda tidak mempunyai kebenaran untuk menggunakan command ini.")
		return
	}

	if len(allowedUsersStore) == 0 {
		b.sendMessage(chatID, "📋 Tiada user dalam senarai.\n\nSila gunakan /allow <user_id> untuk menambah user.")
		return
	}

	var sb strings.Builder
	sb.WriteString("📋 Senarai Users:\n\n")

	allowedCount := 0
	blockedCount := 0

	for userID, user := range allowedUsersStore {
		status := "✅ Allowed"
		if user.Blocked {
			status = "❌ Blocked"
			blockedCount++
		} else {
			allowedCount++
		}

		username := "@" + user.Username
		if user.Username == "" {
			username = "(tiada)"
		}

		sb.WriteString(fmt.Sprintf("🆔 %d\n👤 %s\n📛 %s %s\n📊 %s\n\n",
			userID, username, user.FirstName, user.LastName, status))
	}

	sb.WriteString(fmt.Sprintf("📊 Jumlah: ✅ %d | ❌ %d", allowedCount, blockedCount))
	b.sendMessage(chatID, sb.String())
}

func (b *TelegramBot) isAdmin(chatID int64) bool {
	if len(b.allowedUsers) == 0 {
		return true
	}
	return b.allowedUsers[chatID]
}

func saveAllowedUsers() {
	data, err := json.MarshalIndent(allowedUsersStore, "", "  ")
	if err != nil {
		fmt.Printf("Error saving allowed users: %v\n", err)
		return
	}

	homeDir, _ := os.UserHomeDir()
	configDir := filepath.Join(homeDir, ".config", "terminal-ai")
	os.MkdirAll(configDir, 0755)

	err = os.WriteFile(filepath.Join(configDir, "telegram_users.json"), data, 0644)
	if err != nil {
		fmt.Printf("Error saving allowed users: %v\n", err)
	}
}

func loadAllowedUsers() {
	homeDir, _ := os.UserHomeDir()
	configFile := filepath.Join(homeDir, ".config", "terminal-ai", "telegram_users.json")

	data, err := os.ReadFile(configFile)
	if err != nil {
		return
	}

	var users map[int64]*TelegramUser
	if err := json.Unmarshal(data, &users); err != nil {
		return
	}

	allowedUsersStore = users
}
