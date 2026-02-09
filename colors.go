package main

import "fmt"

const (
	ColorReset   = "\033[0m"
	ColorRed     = "\033[31m"
	ColorGreen   = "\033[32m"
	ColorYellow  = "\033[33m"
	ColorBlue    = "\033[34m"
	ColorMagenta = "\033[35m"
	ColorCyan    = "\033[36m"
	ColorWhite   = "\033[37m"
	ColorBold    = "\033[1m"
	ColorDim     = "\033[2m"
)

func colorSuccess(msg string) string {
	return fmt.Sprintf("%s%s%s", ColorGreen, msg, ColorReset)
}

func colorError(msg string) string {
	return fmt.Sprintf("%s%s%s", ColorRed, msg, ColorReset)
}

func colorWarning(msg string) string {
	return fmt.Sprintf("%s%s%s", ColorYellow, msg, ColorReset)
}

func colorInfo(msg string) string {
	return fmt.Sprintf("%s%s%s", ColorBlue, msg, ColorReset)
}

func colorMagenta(msg string) string {
	return fmt.Sprintf("%s%s%s", ColorMagenta, msg, ColorReset)
}

func colorCyan(msg string) string {
	return fmt.Sprintf("%s%s%s", ColorCyan, msg, ColorReset)
}

func colorBold(msg string) string {
	return fmt.Sprintf("%s%s%s", ColorBold, msg, ColorReset)
}

var CheckMark = colorSuccess("✅")
var CrossMark = colorError("❌")
var WarningMark = colorWarning("⚠️")
var InfoMark = colorInfo("ℹ️")
var FileIcon = colorCyan("📄")
var WebIcon = colorMagenta("🌐")
var SearchIcon = colorCyan("🔍")
var AddIcon = colorSuccess("➕")
var UpdateIcon = colorWarning("🔄")
var TrashIcon = colorError("🗑️")
var StatsIcon = colorInfo("📊")
var ListIcon = colorInfo("📋")
var ClockIcon = colorWarning("🕐")
var DatabaseIcon = colorInfo("💾")
