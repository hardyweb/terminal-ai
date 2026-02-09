package main

import (
	"fmt"
	"strings"
	"time"
)

type SyntaxTheme struct {
	Keyword  string
	String   string
	Number   string
	Comment  string
	Function string
	Type     string
	Reset    string
}

var (
	ThemeDark = SyntaxTheme{
		Keyword:  "\033[1;34m", // Bold Blue
		String:   "\033[0;32m", // Green
		Number:   "\033[0;33m", // Yellow
		Comment:  "\033[0;90m", // Gray
		Function: "\033[1;33m", // Bold Yellow
		Type:     "\033[0;36m", // Cyan
		Reset:    "\033[0m",
	}
	ThemeLight = SyntaxTheme{
		Keyword:  "\033[1;34m",
		String:   "\033[0;32m",
		Number:   "\033[0;33m",
		Comment:  "\033[0;90m",
		Function: "\033[1;33m",
		Type:     "\033[0;36m",
		Reset:    "\033[0m",
	}
)

func HighlightCode(code string, language string) string {
	theme := ThemeDark

	code = strings.TrimSpace(code)

	keywords := []string{
		"func", "return", "if", "else", "for", "range", "switch", "case",
		"default", "import", "package", "var", "const", "type", "struct",
		"interface", "map", "chan", "go", "defer", "select", "class",
		"def", "function", "async", "await", "try", "catch", "finally",
		"throw", "new", "this", "super", "extends", "implements",
		"public", "private", "protected", "static", "void", "int",
		"string", "bool", "boolean", "float", "double", "long",
	}

	for _, kw := range keywords {
		code = strings.ReplaceAll(code, " "+kw+" ", " "+theme.Keyword+kw+theme.Reset+" ")
		code = strings.ReplaceAll(code, "\n"+kw+"\n", "\n"+theme.Keyword+kw+theme.Reset+"\n")
	}

	code = strings.ReplaceAll(code, " true", " "+theme.Number+"true"+theme.Reset)
	code = strings.ReplaceAll(code, " false", " "+theme.Number+"false"+theme.Reset)
	code = strings.ReplaceAll(code, " nil", " "+theme.Number+"nil"+theme.Reset)

	code = highlightStrings(code, theme)

	code = highlightComments(code, theme)

	return code
}

func highlightStrings(code string, theme SyntaxTheme) string {
	result := ""
	i := 0

	for i < len(code) {
		c := code[i]

		if i+1 < len(code) && c == '\\' {
			result += string(c)
			i++
			if i < len(code) {
				result += string(code[i])
				i++
			}
			continue
		}

		if c == '"' || c == '\'' || c == '`' {
			result += theme.String + string(c)
			i++
			for i < len(code) && code[i] != c {
				if code[i] == '\\' && i+1 < len(code) {
					result += string(code[i])
					i++
				}
				if i < len(code) {
					result += string(code[i])
					i++
				}
			}
			if i < len(code) {
				result += string(code[i]) + theme.Reset
				i++
			}
		} else {
			result += string(c)
			i++
		}
	}

	return result
}

func highlightComments(code string, theme SyntaxTheme) string {
	result := ""
	i := 0

	for i < len(code) {
		if i+1 < len(code) && code[i] == '/' && code[i+1] == '/' {
			result += theme.Comment
			for i < len(code) && code[i] != '\n' {
				result += string(code[i])
				i++
			}
			result += theme.Reset
		} else if i+1 < len(code) && code[i] == '/' && code[i+1] == '*' {
			result += theme.Comment
			result += string(code[i])
			i++
			result += string(code[i])
			i++
			for i+1 < len(code) && !(code[i] == '*' && code[i+1] == '/') {
				result += string(code[i])
				i++
			}
			if i < len(code) {
				result += string(code[i])
				i++
			}
			if i < len(code) {
				result += string(code[i])
				i++
			}
			result += theme.Reset
		} else {
			result += string(code[i])
			i++
		}
	}

	return result
}

func FormatCodeBlock(code string, language string) string {
	highlighted := HighlightCode(code, language)

	border := "```"
	if language != "" {
		border = "```" + language
	}

	return fmt.Sprintf("%s\n%s\n%s", border, highlighted, "```")
}

type ProgressBar struct {
	Total     int
	Current   int
	Width     int
	CharFill  string
	CharEmpty string
	Color     string
}

func NewProgressBar(total int) *ProgressBar {
	return &ProgressBar{
		Total:     total,
		Current:   0,
		Width:     30,
		CharFill:  "█",
		CharEmpty: "░",
		Color:     ColorCyan,
	}
}

func (p *ProgressBar) Update(current int) {
	p.Current = current
}

func (p *ProgressBar) Add(n int) {
	p.Current += n
	if p.Current > p.Total {
		p.Current = p.Total
	}
}

func (p *ProgressBar) String() string {
	if p.Total == 0 {
		return p.Color + strings.Repeat(p.CharEmpty, p.Width) + ColorReset
	}

	percent := float64(p.Current) / float64(p.Total)
	fillWidth := int(float64(p.Width) * percent)

	fill := strings.Repeat(p.CharFill, fillWidth)
	empty := strings.Repeat(p.CharEmpty, p.Width-fillWidth)

	percentStr := fmt.Sprintf("%.0f%%", percent*100)

	return p.Color + fill + empty + ColorReset + " " + percentStr
}

var Spinners = []string{
	"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏",
}

type Spinner struct {
	Message string
	Index   int
	Running bool
}

func NewSpinner(message string) *Spinner {
	return &Spinner{
		Message: message,
		Index:   0,
		Running: true,
	}
}

func (s *Spinner) Update() {
	if !s.Running {
		return
	}
	s.Index = (s.Index + 1) % len(Spinners)
}

func (s *Spinner) String() string {
	return fmt.Sprintf("%s %s", ColorCyan+Spinners[s.Index]+ColorReset, s.Message)
}

func (s *Spinner) Stop() {
	s.Running = false
	fmt.Printf("\r%s %s\n", colorSuccess("✓"), s.Message)
}

func (s *Spinner) Fail() {
	s.Running = false
	fmt.Printf("\r%s %s\n", colorError("✗"), s.Message)
}

type Table struct {
	Headers []string
	Rows    [][]string
	Aligns  []string
}

func NewTable(headers []string) *Table {
	return &Table{
		Headers: headers,
		Rows:    make([][]string, 0),
		Aligns:  make([]string, len(headers)),
	}
}

func (t *Table) AddRow(row []string) {
	t.Rows = append(t.Rows, row)
}

func (t *Table) SetAlign(index int, align string) {
	if index >= 0 && index < len(t.Aligns) {
		t.Aligns[index] = align
	}
}

func (t *Table) String() string {
	if len(t.Headers) == 0 {
		return ""
	}

	colWidths := make([]int, len(t.Headers))
	for i, header := range t.Headers {
		colWidths[i] = len(header)
	}

	for _, row := range t.Rows {
		for i, cell := range row {
			if len(cell) > colWidths[i] {
				colWidths[i] = len(cell)
			}
		}
	}

	var sb strings.Builder

	borderTop := "┌"
	borderSep := "├"
	borderBottom := "└"
	for i, width := range colWidths {
		borderTop += strings.Repeat("─", width+2)
		borderSep += strings.Repeat("─", width+2)
		borderBottom += strings.Repeat("─", width+2)
		if i < len(colWidths)-1 {
			borderTop += "┬"
			borderSep += "┼"
			borderBottom += "┴"
		}
	}
	borderTop += "┐"
	borderSep += "┤"
	borderBottom += "┘"

	sb.WriteString(ColorBold + borderTop + ColorReset + "\n")

	headerRow := "│"
	for i, header := range t.Headers {
		headerRow += " " + ColorBold + header + ColorReset
		headerRow += strings.Repeat(" ", colWidths[i]-len(header)+1) + "│"
	}
	sb.WriteString(headerRow + "\n")

	sb.WriteString(ColorBold + borderSep + ColorReset + "\n")

	for _, row := range t.Rows {
		rowStr := "│"
		for i, cell := range row {
			padding := colWidths[i] - len(cell)
			if t.Aligns[i] == "right" {
				rowStr += strings.Repeat(" ", padding+1) + cell + "│"
			} else {
				rowStr += " " + cell + strings.Repeat(" ", padding+1) + "│"
			}
		}
		sb.WriteString(rowStr + "\n")
	}

	sb.WriteString(ColorBold + borderBottom + ColorReset + "\n")

	return sb.String()
}

func FormatBox(content string, title string) string {
	lines := strings.Split(content, "\n")

	maxLen := 0
	for _, line := range lines {
		if len(line) > maxLen {
			maxLen = len(line)
		}
	}
	if len(title) > maxLen {
		maxLen = len(title)
	}

	border := "┌" + strings.Repeat("─", maxLen+2) + "┐"
	titleLine := ""
	if title != "" {
		titleLine = "│ " + ColorBold + title + ColorReset
		titleLine += strings.Repeat(" ", maxLen-len(title)) + " │\n"
	}

	var sb strings.Builder
	sb.WriteString(ColorBold + border + ColorReset + "\n")
	if titleLine != "" {
		sb.WriteString(titleLine)
		sb.WriteString("├" + strings.Repeat("─", maxLen+2) + "┤\n")
	}

	for _, line := range lines {
		sb.WriteString("│ " + line)
		sb.WriteString(strings.Repeat(" ", maxLen-len(line)) + " │\n")
	}

	sb.WriteString(ColorBold + "└" + strings.Repeat("─", maxLen+2) + "┘" + ColorReset)

	return sb.String()
}

func FormatSection(title string, content string) string {
	var sb strings.Builder
	sb.WriteString(ColorBold + "\n═══ " + title + " " + strings.Repeat("═", 60-len(title)-3) + ColorReset + "\n")
	sb.WriteString(content)
	sb.WriteString("\n")
	return sb.String()
}

func FormatBulletList(items []string) string {
	var sb strings.Builder
	for i, item := range items {
		sb.WriteString(ColorCyan + "• " + ColorReset + item)
		if i < len(items)-1 {
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

func FormatNumberedList(items []string) string {
	var sb strings.Builder
	for i, item := range items {
		sb.WriteString(ColorCyan + fmt.Sprintf("%d. ", i+1) + ColorReset + item)
		if i < len(items)-1 {
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

func FormatDivider() string {
	return ColorDim + strings.Repeat("─", 60) + ColorReset
}

func FormatSeparator() string {
	return ColorDim + strings.Repeat("─", 40) + ColorReset
}

func TypewriterText(text string, delay time.Duration) {
	for _, char := range text {
		fmt.Print(string(char))
		time.Sleep(delay)
	}
	fmt.Println()
}

func PrintLoading(message string, duration time.Duration) {
	spinner := NewSpinner(message)
	for end := time.Now().Add(duration); time.Now().Before(end); {
		fmt.Printf("\r%s", spinner.String())
		spinner.Update()
		time.Sleep(100 * time.Millisecond)
	}
	spinner.Stop()
}

func PrintSuccess(message string) {
	fmt.Printf("%s %s\n", colorSuccess("✓"), message)
}

func PrintError(message string) {
	fmt.Printf("%s %s\n", colorError("✗"), message)
}

func PrintWarning(message string) {
	fmt.Printf("%s %s\n", colorWarning("⚠"), message)
}

func PrintInfo(message string) {
	fmt.Printf("%s %s\n", colorInfo("ℹ"), message)
}
