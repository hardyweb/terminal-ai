package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	aiStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(lipgloss.Color("#553C9A")).
		Padding(0, 2).
		MarginLeft(2)

	userStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(lipgloss.Color("#4A5568")).
			Padding(0, 2).
			MarginRight(2)

	panelBorderColor = lipgloss.Color("#4A5568")
	borderStyle      = lipgloss.RoundedBorder()
)

type chatModel struct {
	messages    []string
	currentMsg  string
	input       string
	provider    string
	width       int
	height      int
	streaming   bool
	program     *tea.Program
	mu          sync.Mutex
	autoSend    bool
	autoMessage string
}

func newChatModel(provider string, message string, autoSend bool) *chatModel {
	messages := []string{}
	if message != "" {
		messages = append(messages, fmt.Sprintf("👤 YOU: %s", message))
	}
	return &chatModel{
		messages:    messages,
		provider:    provider,
		streaming:   false,
		autoSend:    autoSend,
		autoMessage: message,
	}
}

func (m *chatModel) Init() tea.Cmd {
	if m.autoSend && m.autoMessage != "" {
		m.streaming = true
		go func() {
			streamAI(m.provider, m.autoMessage, func(chunk string) {
				if m.program != nil {
					m.program.Send(streamingMsg{chunk: chunk, complete: false})
				}
			}, func() {
				if m.program != nil {
					m.program.Send(streamingMsg{chunk: "", complete: true})
				}
			})
		}()
	}
	return nil
}

func (m *chatModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case streamingMsg:
		m.mu.Lock()
		if msg.complete {
			if m.currentMsg != "" {
				m.messages = append(m.messages, fmt.Sprintf("🤖 AI: %s", m.currentMsg))
			}
			m.currentMsg = ""
			m.streaming = false
		} else {
			m.currentMsg += msg.chunk
		}
		m.mu.Unlock()
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" || msg.String() == "esc" {
			return m, func() tea.Msg {
				return tea.QuitMsg{}
			}
		}

		if m.streaming {
			return m, nil
		}

		if msg.String() == "enter" && m.input != "" {
			message := m.input
			m.input = ""
			m.messages = append(m.messages, fmt.Sprintf("👤 YOU: %s", message))
			m.currentMsg = ""
			m.streaming = true

			go func() {
				streamAI(m.provider, message, func(chunk string) {
					if m.program != nil {
						m.program.Send(streamingMsg{chunk: chunk, complete: false})
					}
				}, func() {
					if m.program != nil {
						m.program.Send(streamingMsg{chunk: "", complete: true})
					}
				})
			}()
			return m, nil
		}

		if msg.String() == "backspace" {
			if len(m.input) > 0 {
				m.input = m.input[:len(m.input)-1]
			}
		} else if msg.String() == "space" {
			m.input += " "
		} else if len(msg.Runes) > 0 {
			m.input += string(msg.Runes[0])
		}
	}

	return m, nil
}

type streamingMsg struct {
	chunk    string
	complete bool
}

func (m *chatModel) View() string {
	if m.width == 0 {
		m.width = 80
		m.height = 24
	}

	isVertical := m.width < 60
	panelBorder := lipgloss.NewStyle().
		Border(borderStyle).
		BorderForeground(panelBorderColor).
		Padding(1)

	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#E2E8F0")).
		Background(lipgloss.Color("#2D3748")).
		Padding(0, 2)

	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#A0AEC0")).
		Background(lipgloss.Color("#1A202C")).
		Padding(0, 1)

	statusIndicator := ""
	if m.streaming {
		statusIndicator = " 💭"
	}

	chatArea := m.renderChatArea()
	historyArea := m.renderHistoryArea()

	titleBar := lipgloss.JoinHorizontal(
		lipgloss.Top,
		lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FAFAFA")).
			Background(lipgloss.Color("#7D56F4")).
			Padding(0, 2).
			Render(" Terminal AI "),
		lipgloss.NewStyle().
			Foreground(lipgloss.Color("#E2E8F0")).
			Background(lipgloss.Color("#2D3748")).
			Padding(0, 2).
			Render(fmt.Sprintf(" Provider: %s ", m.provider)),
	)

	if isVertical {
		return m.renderVerticalLayout(titleBar, chatArea, historyArea, statusIndicator, panelBorder, headerStyle, helpStyle)
	}
	return m.renderHorizontalLayout(titleBar, chatArea, historyArea, statusIndicator, panelBorder, headerStyle, helpStyle)
}

func (m *chatModel) renderHorizontalLayout(titleBar, chatArea, historyArea, statusIndicator string, panelBorder, headerStyle, helpStyle lipgloss.Style) string {
	chatWidth := int(float64(m.width-6) * 0.6)
	historyWidth := m.width - 6 - chatWidth

	chatPanel := panelBorder.
		Width(chatWidth).
		Render(
			lipgloss.JoinVertical(
				lipgloss.Left,
				headerStyle.Render("🤖 AI Chat"),
				chatArea,
				helpStyle.Render(fmt.Sprintf("▌ Type:%s%s_", statusIndicator, m.input)),
			),
		)

	historyPanel := panelBorder.
		Width(historyWidth).
		Render(
			lipgloss.JoinVertical(
				lipgloss.Left,
				headerStyle.Render("👤 Chat History"),
				historyArea,
			),
		)

	mainContent := lipgloss.JoinHorizontal(
		lipgloss.Top,
		chatPanel,
		historyPanel,
	)

	helpBar := helpStyle.Render(" [Enter] Send | [Esc/Ctrl+C] Quit ")

	return lipgloss.JoinVertical(
		lipgloss.Left,
		titleBar,
		mainContent,
		helpBar,
	)
}

func (m *chatModel) renderVerticalLayout(titleBar, chatArea, historyArea, statusIndicator string, panelBorder, headerStyle, helpStyle lipgloss.Style) string {
	chatHeight := int(float64(m.height-7) * 0.6)
	historyHeight := m.height - 7 - chatHeight

	chatPanel := panelBorder.
		Height(chatHeight).
		Width(m.width - 4).
		Render(
			lipgloss.JoinVertical(
				lipgloss.Left,
				headerStyle.Render("🤖 AI Chat"),
				chatArea,
				helpStyle.Render(fmt.Sprintf("▌ Type:%s%s_", statusIndicator, m.input)),
			),
		)

	historyPanel := panelBorder.
		Height(historyHeight).
		Width(m.width - 4).
		Render(
			lipgloss.JoinVertical(
				lipgloss.Left,
				headerStyle.Render("👤 Chat History"),
				historyArea,
			),
		)

	mainContent := lipgloss.JoinVertical(
		lipgloss.Left,
		chatPanel,
		historyPanel,
	)

	helpBar := helpStyle.Render(" [Enter] Send | [Esc/Ctrl+C] Quit ")

	return lipgloss.JoinVertical(
		lipgloss.Left,
		titleBar,
		mainContent,
		helpBar,
	)
}

func (m *chatModel) renderChatArea() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	content := ""
	for _, msg := range m.messages {
		if strings.HasPrefix(msg, "👤 YOU:") {
			content += userStyle.Render(msg) + "\n"
		} else {
			content += aiStyle.Render(msg) + "\n"
		}
	}
	if m.currentMsg != "" {
		content += aiStyle.Render("🤖 AI: "+m.currentMsg) + "\n"
	}
	if content == "" {
		content = "\n\n Start chatting...\n\n"
	}
	return content
}

func (m *chatModel) renderHistoryArea() string {
	sessions := listSessions()
	if len(sessions) == 0 {
		return "\n No sessions yet\n"
	}

	content := ""
	for i, s := range sessions {
		preview := s.Title
		if len(preview) > 20 {
			preview = preview[:17] + "..."
		}
		content += fmt.Sprintf("%d. %s\n", i+1, preview)
		content += fmt.Sprintf("   💬 %d msgs\n\n", len(s.Messages))
	}
	return content
}

type StreamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
}

func streamAI(provider, message string, onChunk func(string), onComplete func()) {
	defer onComplete()

	providerCfg := providers[provider]

	if providerCfg.Endpoint == "" {
		switch provider {
		case "openrouter":
			providerCfg.Endpoint = "https://openrouter.ai/api/v1/chat/completions"
		case "gemini":
			providerCfg.Endpoint = "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent"
		case "groq":
			providerCfg.Endpoint = "https://api.groq.com/openai/v1/chat/completions"
		}
	}

	if providerCfg.Model == "" {
		switch provider {
		case "openrouter":
			providerCfg.Model = "meta-llama/llama-3.2-3b-instruct:free"
		case "gemini":
			providerCfg.Model = "gemini-2.0-flash"
		case "groq":
			providerCfg.Model = "llama-3.3-70b-versatile"
		}
	}

	if providerCfg.APIKey == "" {
		onChunk("Error: API key not configured.")
		return
	}

	reqBody := fmt.Sprintf(`{"model":"%s","messages":[{"role":"user","content":"%s"}],"stream":true}`,
		providerCfg.Model, strings.ReplaceAll(message, `"`, `\"`))

	client := &http.Client{Timeout: 300 * time.Second}
	req, err := http.NewRequest("POST", providerCfg.Endpoint, strings.NewReader(reqBody))
	if err != nil {
		onChunk(fmt.Sprintf("Error: %v", err))
		return
	}

	req.Header.Set("Content-Type", "application/json")
	if provider == "openrouter" {
		req.Header.Set("Authorization", "Bearer "+providerCfg.APIKey)
		req.Header.Set("HTTP-Referer", "https://terminal-ai.local")
		req.Header.Set("X-Title", "Terminal AI CLI")
	} else if provider == "gemini" {
		req.Header.Set("x-goog-api-key", providerCfg.APIKey)
	} else {
		req.Header.Set("Authorization", "Bearer "+providerCfg.APIKey)
	}

	resp, err := client.Do(req)
	if err != nil {
		onChunk(fmt.Sprintf("Error: %v", err))
		return
	}
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			break
		}

		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var chunk StreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
			onChunk(chunk.Choices[0].Delta.Content)
		}
	}
}

func startTUIChat(provider string, message string) {
	autoSend := message != ""
	model := newChatModel(provider, message, autoSend)
	p := tea.NewProgram(model, tea.WithAltScreen())
	model.program = p
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running TUI: %v\n", err)
	}
}
