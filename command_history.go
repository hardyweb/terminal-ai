package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type CommandHistory struct {
	HistoryDir  string
	HistoryFile string
	MaxEntries  int
	entries     []string
}

var history *CommandHistory

func InitHistory() error {
	homeDir, _ := os.UserHomeDir()
	historyDir := filepath.Join(homeDir, ".config", "terminal-ai", "history")

	if err := os.MkdirAll(historyDir, 0755); err != nil {
		return err
	}

	history = &CommandHistory{
		HistoryDir:  historyDir,
		HistoryFile: filepath.Join(historyDir, "command_history.txt"),
		MaxEntries:  1000,
		entries:     make([]string, 0),
	}

	err := history.Load()
	if err != nil {
		if os.IsNotExist(err) {
			history.Save()
			return nil
		}
		return err
	}

	return nil
}

func NewCommandHistory() (*CommandHistory, error) {
	h := &CommandHistory{
		HistoryDir:  filepath.Join(os.Getenv("HOME"), ".config", "terminal-ai", "history"),
		HistoryFile: filepath.Join(os.Getenv("HOME"), ".config", "terminal-ai", "history", "command_history.txt"),
		MaxEntries:  1000,
		entries:     make([]string, 0),
	}

	if err := os.MkdirAll(h.HistoryDir, 0755); err != nil {
		return nil, err
	}

	if err := h.Load(); err != nil {
		return nil, err
	}

	return h, nil
}

func (h *CommandHistory) Load() error {
	file, err := os.Open(h.HistoryFile)
	if err != nil {
		if os.IsNotExist(err) {
			h.entries = make([]string, 0)
			return nil
		}
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			h.entries = append(h.entries, line)
		}
	}

	return scanner.Err()
}

func (h *CommandHistory) Save() error {
	file, err := os.Create(h.HistoryFile)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	for _, entry := range h.entries {
		fmt.Fprintln(writer, entry)
	}

	return writer.Flush()
}

func (h *CommandHistory) Add(command string) {
	command = strings.TrimSpace(command)
	if command == "" {
		return
	}

	if len(h.entries) > 0 && h.entries[len(h.entries)-1] == command {
		return
	}

	h.entries = append(h.entries, command)

	for len(h.entries) > h.MaxEntries {
		h.entries = h.entries[1:]
	}

	h.Save()
}

func (h *CommandHistory) GetAll() []string {
	result := make([]string, len(h.entries))
	copy(result, h.entries)
	return result
}

func (h *CommandHistory) GetRecent(count int) []string {
	if count > len(h.entries) {
		count = len(h.entries)
	}
	result := make([]string, count)
	copy(result, h.entries[len(h.entries)-count:])
	return result
}

func (h *CommandHistory) Search(prefix string) []string {
	var results []string
	prefix = strings.ToLower(prefix)

	for i := len(h.entries) - 1; i >= 0; i-- {
		if strings.HasPrefix(strings.ToLower(h.entries[i]), prefix) {
			results = append(results, h.entries[i])
			if len(results) >= 10 {
				break
			}
		}
	}

	return results
}

func (h *CommandHistory) Clear() error {
	h.entries = make([]string, 0)
	return h.Save()
}

func (h *CommandHistory) RemoveDuplicates() {
	seen := make(map[string]bool)
	var unique []string

	for _, entry := range h.entries {
		if !seen[entry] {
			seen[entry] = true
			unique = append(unique, entry)
		}
	}

	h.entries = unique
	h.Save()
}

type HistoryEntry struct {
	Command   string
	Timestamp time.Time
	Duration  time.Duration
	Success   bool
}

func (h *CommandHistory) AddWithMetadata(command string, duration time.Duration, success bool) {
	h.Add(command)
}

func HandleCommandHistory() {
	if history == nil {
		if err := InitHistory(); err != nil {
			fmt.Printf("%s Failed to initialize history: %v\n", colorError("❌"), err)
			os.Exit(1)
		}
	}

	if len(os.Args) < 3 {
		showHistoryHelp()
		os.Exit(1)
	}

	subCmd := os.Args[2]

	switch subCmd {
	case "list":
		handleHistoryList()
	case "clear":
		handleHistoryClear()
	case "search":
		handleHistorySearch()
	case "recent":
		handleHistoryRecent()
	case "dedup":
		handleHistoryDedup()
	default:
		showHistoryHelp()
	}
}

func handleHistoryList() {
	entries := history.GetAll()

	if len(entries) == 0 {
		fmt.Printf("\n%s No command history yet.\n", colorInfo("ℹ"))
		fmt.Println("Start using terminal-ai to build your command history!")
		return
	}

	fmt.Printf("\n%s Command History (%d entries)\n", colorCyan("📜"), len(entries))
	fmt.Println(strings.Repeat("═", 60))

	table := NewTable([]string{"#", "Command", "Time"})
	table.SetAlign(0, "right")
	table.SetAlign(2, "left")

	for i, entry := range entries {
		timestamp := time.Now()
		table.AddRow([]string{
			fmt.Sprintf("%d", i+1),
			truncateMiddle(entry, 50),
			timestamp.Format("15:04"),
		})
	}

	fmt.Print(table.String())
}

func handleHistoryClear() {
	fmt.Printf("%s Clear all command history? [y/N]: ", colorWarning("⚠"))
	var confirm string
	fmt.Scanln(&confirm)

	if confirm != "y" && confirm != "Y" {
		fmt.Println("Cancelled.")
		return
	}

	if err := history.Clear(); err != nil {
		fmt.Printf("%s Failed to clear history: %v\n", colorError("❌"), err)
		os.Exit(1)
	}

	fmt.Printf("%s Command history cleared.\n", colorSuccess("✅"))
}

func handleHistorySearch() {
	if len(os.Args) < 4 {
		fmt.Println(colorError("Usage: terminal-ai history search <prefix>"))
		os.Exit(1)
	}

	prefix := strings.Join(os.Args[3:], " ")
	results := history.Search(prefix)

	fmt.Printf("\n%s Searching history for: %s\n", colorCyan("🔍"), colorBold(prefix))
	fmt.Println(strings.Repeat("═", 60))

	if len(results) == 0 {
		fmt.Printf("%s No matching commands found.\n", colorInfo("ℹ"))
		return
	}

	for i, entry := range results {
		fmt.Printf("%d. %s\n", i+1, entry)
	}
}

func handleHistoryRecent() {
	count := 10
	if len(os.Args) >= 4 {
		fmt.Sscanf(os.Args[3], "%d", &count)
	}

	entries := history.GetRecent(count)

	fmt.Printf("\n%s Recent Commands (%d)\n", colorCyan("🕐"), len(entries))
	fmt.Println(strings.Repeat("═", 60))

	for i, entry := range entries {
		fmt.Printf("%d. %s\n", i+1, entry)
	}
}

func handleHistoryDedup() {
	history.RemoveDuplicates()
	fmt.Printf("%s Duplicates removed.\n", colorSuccess("✅"))
}

func showHistoryHelp() {
	fmt.Println(colorBold("Command History Commands:"))
	fmt.Println()
	fmt.Printf("  %s %s  List all commands in history\n", colorCyan("terminal-ai"), colorBold("history list"))
	fmt.Printf("  %s %s  Search history by prefix\n", colorCyan("terminal-ai"), colorBold("history search <prefix>"))
	fmt.Printf("  %s %s  Show recent commands\n", colorCyan("terminal-ai"), colorBold("history recent [count]"))
	fmt.Printf("  %s %s  Remove duplicates\n", colorCyan("terminal-ai"), colorBold("history dedup"))
	fmt.Printf("  %s %s  Clear all history\n", colorCyan("terminal-ai"), colorBold("history clear"))
	fmt.Println()
	fmt.Println("History is automatically saved after each command.")
}

func truncateMiddle(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	half := (maxLen - 3) / 2
	return s[:half] + "..." + s[len(s)-half:]
}

func AddToHistory(command string) {
	if history == nil {
		InitHistory()
	}
	if history != nil {
		history.Add(command)
	}
}
