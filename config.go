package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ConfigManager struct {
	configDir  string
	configFile string
	config     map[string]string
}

var configManager *ConfigManager

func InitConfigManager() error {
	homeDir, _ := os.UserHomeDir()
	configDir := filepath.Join(homeDir, ".config", "terminal-ai")

	if err := os.MkdirAll(configDir, 0755); err != nil {
		return err
	}

	configManager = &ConfigManager{
		configDir:  configDir,
		configFile: filepath.Join(configDir, "config.json"),
		config:     make(map[string]string),
	}

	return configManager.Load()
}

func NewConfigManager() (*ConfigManager, error) {
	homeDir, _ := os.UserHomeDir()
	configDir := filepath.Join(homeDir, ".config", "terminal-ai")

	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, err
	}

	cm := &ConfigManager{
		configDir:  configDir,
		configFile: filepath.Join(configDir, "config.json"),
		config:     make(map[string]string),
	}

	if err := cm.Load(); err != nil {
		return nil, err
	}

	return cm, nil
}

func (cm *ConfigManager) Load() error {
	data, err := os.ReadFile(cm.configFile)
	if err != nil {
		if os.IsNotExist(err) {
			cm.config = make(map[string]string)
			return nil
		}
		return err
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			cm.config[key] = value
		}
	}

	return nil
}

func (cm *ConfigManager) Save() error {
	var lines []string

	for key, value := range cm.config {
		lines = append(lines, fmt.Sprintf("%s=%s", key, value))
	}

	content := strings.Join(lines, "\n")
	return os.WriteFile(cm.configFile, []byte(content), 0644)
}

func (cm *ConfigManager) Get(key string) string {
	return cm.config[key]
}

func (cm *ConfigManager) Set(key, value string) error {
	cm.config[key] = value
	return cm.Save()
}

func (cm *ConfigManager) Unset(key string) error {
	delete(cm.config, key)
	return cm.Save()
}

func (cm *ConfigManager) List() map[string]string {
	result := make(map[string]string)
	for k, v := range cm.config {
		result[k] = v
	}
	return result
}

func (cm *ConfigManager) Reset() error {
	cm.config = make(map[string]string)
	return cm.Save()
}

func HandleConfigCommand() {
	if len(os.Args) < 3 {
		showConfigHelp()
		os.Exit(1)
	}

	subCmd := os.Args[2]

	if configManager == nil {
		if err := InitConfigManager(); err != nil {
			fmt.Printf("%s Failed to initialize config: %v\n", colorError("❌"), err)
			os.Exit(1)
		}
	}

	switch subCmd {
	case "list":
		handleConfigList()
	case "get":
		handleConfigGet()
	case "set":
		handleConfigSet()
	case "unset":
		handleConfigUnset()
	case "reset":
		handleConfigReset()
	case "edit":
		handleConfigEdit()
	default:
		showConfigHelp()
	}
}

func handleConfigList() {
	config := configManager.List()

	fmt.Printf("\n%s Terminal AI Configuration\n", colorCyan("⚙"))
	fmt.Println(strings.Repeat("═", 60))

	table := NewTable([]string{"Setting", "Value"})
	table.SetAlign(1, "left")

	if len(config) == 0 {
		fmt.Printf("\n%s No configuration set.\n", colorInfo("ℹ"))
		fmt.Println("Use 'terminal-ai config set <key>=<value>' to set values.")
	} else {
		for key, value := range config {
			displayValue := value
			if isSensitive(key) {
				displayValue = "********"
			}
			table.AddRow([]string{colorBold(key), displayValue})
		}
		fmt.Print(table.String())
	}

	fmt.Println()
	fmt.Println(colorBold("Common Settings:"))
	fmt.Println("  OPENROUTER_API_KEY      - API key for OpenRouter")
	fmt.Println("  GEMINI_API_KEY          - API key for Google Gemini")
	fmt.Println("  GROQ_API_KEY             - API key for Groq")
	fmt.Println("  DEFAULT_PROVIDER         - Default AI provider")
	fmt.Println("  USE_OLLAMA_EMBEDDINGS   - Use local Ollama embeddings")
	fmt.Println("  OLLAMA_EMBEDDINGS_URL    - Ollama server URL")
	fmt.Println("  EMBEDDINGS_MODEL         - Embeddings model name")
}

func handleConfigGet() {
	if len(os.Args) < 4 {
		fmt.Println(colorError("Usage: terminal-ai config get <key>"))
		os.Exit(1)
	}

	key := os.Args[3]
	value := configManager.Get(key)

	if value == "" {
		fmt.Printf("%s '%s' is not set.\n", colorWarning("⚠"), key)
		os.Exit(1)
	}

	if isSensitive(key) {
		value = "********"
	}

	fmt.Printf("%s=%s\n", key, value)
}

func handleConfigSet() {
	if len(os.Args) < 4 {
		fmt.Println(colorError("Usage: terminal-ai config set <key>=<value>"))
		fmt.Println(colorInfo("Example: terminal-ai config set DEFAULT_PROVIDER=groq"))
		os.Exit(1)
	}

	line := strings.Join(os.Args[3:], "=")
	parts := strings.SplitN(line, "=", 2)

	if len(parts) != 2 {
		fmt.Println(colorError("Usage: terminal-ai config set <key>=<value>"))
		os.Exit(1)
	}

	key := strings.TrimSpace(parts[0])
	value := strings.TrimSpace(parts[1])

	if key == "" {
		fmt.Println(colorError("Key cannot be empty"))
		os.Exit(1)
	}

	if err := configManager.Set(key, value); err != nil {
		fmt.Printf("%s Failed to save config: %v\n", colorError("❌"), err)
		os.Exit(1)
	}

	if isSensitive(key) {
		fmt.Printf("%s %s=******** saved.\n", colorSuccess("✅"), key)
	} else {
		fmt.Printf("%s %s=%s saved.\n", colorSuccess("✅"), key, value)
	}
}

func handleConfigUnset() {
	if len(os.Args) < 4 {
		fmt.Println(colorError("Usage: terminal-ai config unset <key>"))
		os.Exit(1)
	}

	key := os.Args[3]

	if configManager.Get(key) == "" {
		fmt.Printf("%s '%s' is not set.\n", colorWarning("⚠"), key)
		os.Exit(1)
	}

	if err := configManager.Unset(key); err != nil {
		fmt.Printf("%s Failed to unset config: %v\n", colorError("❌"), err)
		os.Exit(1)
	}

	fmt.Printf("%s '%s' removed.\n", colorSuccess("✅"), key)
}

func handleConfigReset() {
	fmt.Printf("%s Reset all configuration? [y/N]: ", colorWarning("⚠"))
	var confirm string
	fmt.Scanln(&confirm)

	if confirm != "y" && confirm != "Y" {
		fmt.Println("Cancelled.")
		return
	}

	if err := configManager.Reset(); err != nil {
		fmt.Printf("%s Failed to reset config: %v\n", colorError("❌"), err)
		os.Exit(1)
	}

	fmt.Printf("%s Configuration reset to defaults.\n", colorSuccess("✅"))
}

func handleConfigEdit() {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "nano"
	}

	if _, err := os.Stat(configManager.configFile); os.IsNotExist(err) {
		configManager.Save()
	}

	fmt.Printf("%s Opening %s in %s\n", colorInfo("ℹ"), configManager.configFile, editor)
	fmt.Println("Make your changes and save the file.")

	// This would normally execute the editor
	fmt.Printf("\n%s To edit manually, run:\n", colorInfo("ℹ"))
	fmt.Printf("  %s %s\n", editor, configManager.configFile)
}

func showConfigHelp() {
	fmt.Println(colorBold("Configuration Management Commands:"))
	fmt.Println()
	fmt.Printf("  %s %s  List all configuration\n", colorCyan("terminal-ai"), colorBold("config list"))
	fmt.Printf("  %s %s  Get a configuration value\n", colorCyan("terminal-ai"), colorBold("config get <key>"))
	fmt.Printf("  %s %s  Set a configuration value\n", colorCyan("terminal-ai"), colorBold("config set <key>=<value>"))
	fmt.Printf("  %s %s  Unset a configuration\n", colorCyan("terminal-ai"), colorBold("config unset <key>"))
	fmt.Printf("  %s %s  Reset all configuration\n", colorCyan("terminal-ai"), colorBold("config reset"))
	fmt.Printf("  %s %s  Open config in editor\n", colorCyan("terminal-ai"), colorBold("config edit"))
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Printf("  terminal-ai config list\n")
	fmt.Printf("  terminal-ai config get DEFAULT_PROVIDER\n")
	fmt.Printf("  terminal-ai config set DEFAULT_PROVIDER=groq\n")
	fmt.Printf("  terminal-ai config unset GEMINI_API_KEY\n")
}

func isSensitive(key string) bool {
	sensitiveKeys := []string{
		"API_KEY",
		"APIKEY",
		"SECRET",
		"PASSWORD",
		"TOKEN",
		"CREDENTIAL",
		"AUTH",
	}

	keyUpper := strings.ToUpper(key)
	for _, sensitive := range sensitiveKeys {
		if strings.Contains(keyUpper, sensitive) {
			return true
		}
	}

	return false
}
