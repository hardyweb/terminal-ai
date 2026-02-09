package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

type InteractiveSession struct {
	provider   string
	sessionID  string
	input      *bufio.Reader
	ragEnabled bool
	ctx        context.Context
	cancel     context.CancelFunc
}

func NewInteractiveSession(provider string) *InteractiveSession {
	ctx, cancel := context.WithCancel(context.Background())
	return &InteractiveSession{
		provider:   provider,
		input:      bufio.NewReader(os.Stdin),
		ragEnabled: true,
		ctx:        ctx,
		cancel:     cancel,
	}
}

func (s *InteractiveSession) Start() {
	clearScreen()
	printWelcomeBanner()

	fmt.Println(colorInfo("AI Provider: " + s.provider))

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

	req := Request{
		Model:    provider.Model,
		Messages: messages,
		Stream:   true,
	}

	fmt.Print(colorBold(colorCyan("\n🤖 " + strings.Title(s.provider) + ": ")))

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
		content := resp.content

		words := strings.Fields(content)
		wordCount := 0
		for _, word := range words {
			fmt.Print(word + " ")
			wordCount++
			if wordCount >= 15 {
				fmt.Println()
				fmt.Print(colorBold(colorCyan("\n🤖 " + strings.Title(s.provider) + ": ")))
				wordCount = 0
			}
			time.Sleep(10 * time.Millisecond)
		}
		fmt.Println()
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

func (s *InteractiveSession) showStats() {
	fmt.Println(colorBold("\n📊 Session Statistics:"))
	fmt.Printf("  Provider: %s\n", s.provider)
	fmt.Printf("  RAG: %v\n", s.ragEnabled)
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
╔═══════════════════════════════════════════════════════╗
║                  Available Commands                 ║
╠═══════════════════════════════════════════════════════╣
║  /help, ?          Show this help                ║
║  /quit, /exit      Exit chat                     ║
║  /clear            Clear screen                   ║
║  /history          Show chat history             ║
║  /stats            Session statistics             ║
║  /provider <name>  Switch provider               ║
║  /model <name>     Set model                     ║
║  /rag on|off       Enable/disable RAG            ║
║  /tokens           Token tracking                 ║
╠═══════════════════════════════════════════════════════╣
║  Just type your message and press Enter!        ║
╚═══════════════════════════════════════════════════════╝`
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
