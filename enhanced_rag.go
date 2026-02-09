package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"terminal-ai/rag"
)

var ragManager *rag.RAGManager
var hybridEngine *rag.HybridSearchEngine

func initRAGEnhanced() error {
	var err error
	ragManager, err = rag.NewRAGManager()
	if err != nil {
		return err
	}

	hybridEngine = rag.NewHybridSearchEngine(ragManager)
	return nil
}

func handleRAGCommandEnhanced() {
	if len(os.Args) < 3 {
		showEnhancedRAGHelp()
		os.Exit(1)
	}

	subCmd := os.Args[2]

	switch subCmd {
	case "index":
		handleRAGIndexEnhanced()
	case "add":
		handleRAGAdd()
	case "web":
		handleRAGWeb()
	case "search":
		handleRAGSearchEnhanced()
	case "status":
		handleRAGStatus()
	case "list":
		handleRAGList()
	case "remove":
		handleRAGRemove()
	case "clear":
		handleRAGClear()
	default:
		showEnhancedRAGHelp()
	}
}

func handleRAGIndexEnhanced() {
	if len(os.Args) < 4 {
		fmt.Println(colorError("Usage: terminal-ai rag index <dir>"))
		os.Exit(1)
	}

	dir := os.Args[3]

	if err := initRAGEnhanced(); err != nil {
		fmt.Printf("%s Failed to initialize RAG: %v\n", CrossMark, err)
		os.Exit(1)
	}

	fmt.Printf("%s Full reindex of directory: %s\n", InfoMark, dir)

	report, err := ragManager.IndexDirectory(dir)
	if err != nil {
		fmt.Printf("%s Error indexing: %v\n", CrossMark, err)
		os.Exit(1)
	}

	printIndexReport(report)
}

func handleRAGAdd() {
	if len(os.Args) < 4 {
		fmt.Println(colorError("Usage: terminal-ai rag add <dir> [dir2] [dir3]..."))
		fmt.Println(colorInfo("Example: terminal-ai rag add ~/docs ~/notes"))
		os.Exit(1)
	}

	dirs := os.Args[3:]

	if err := initRAGEnhanced(); err != nil {
		fmt.Printf("%s Failed to initialize RAG: %v\n", CrossMark, err)
		os.Exit(1)
	}

	fmt.Printf("%s Incremental update for: %s\n", UpdateIcon, strings.Join(dirs, ", "))

	report, err := ragManager.IndexDirectories(dirs)
	if err != nil {
		fmt.Printf("%s Error indexing: %v\n", CrossMark, err)
		os.Exit(1)
	}

	printAddReport(report)
}

func handleRAGWeb() {
	if len(os.Args) < 4 {
		fmt.Println(colorError("Usage: terminal-ai rag web <url>"))
		os.Exit(1)
	}

	targetURL := os.Args[3]

	if err := initRAGEnhanced(); err != nil {
		fmt.Printf("%s Failed to initialize RAG: %v\n", CrossMark, err)
		os.Exit(1)
	}

	fmt.Printf("%s Scraping: %s\n", WebIcon, targetURL)

	scraper, err := rag.NewWebScraper()
	if err != nil {
		fmt.Printf("%s Failed to create scraper: %v\n", CrossMark, err)
		os.Exit(1)
	}

	ctx := context.Background()
	chunks, err := scraper.ScrapeAndIndex(ctx, targetURL, ragManager)
	if err != nil {
		fmt.Printf("%s Error scraping: %v\n", CrossMark, err)
		os.Exit(1)
	}

	fmt.Printf("%s Successfully indexed!\n", CheckMark)
	fmt.Printf("   URL: %s\n", targetURL)
	fmt.Printf("   Chunks created: %d\n", len(chunks))
}

func handleRAGSearchEnhanced() {
	if len(os.Args) < 4 {
		fmt.Println(colorError("Usage: terminal-ai rag search <query>"))
		os.Exit(1)
	}

	query := strings.Join(os.Args[3:], " ")

	if err := initRAGEnhanced(); err != nil {
		fmt.Printf("%s Failed to initialize RAG: %v\n", CrossMark, err)
		os.Exit(1)
	}

	fmt.Printf("%s Searching: %s\n", SearchIcon, colorBold(query))

	ctx := context.Background()

	results, stats, err := hybridEngine.SearchWithStats(ctx, query)
	if err != nil {
		fmt.Printf("%s Error searching: %v\n", CrossMark, err)
		os.Exit(1)
	}

	printEnhancedSearchResults(results, query, stats.SearchTime)
}

func handleRAGStatus() {
	if err := initRAGEnhanced(); err != nil {
		fmt.Printf("%s Failed to initialize RAG: %v\n", CrossMark, err)
		os.Exit(1)
	}

	stats, err := ragManager.GetStats()
	if err != nil {
		fmt.Printf("%s Error getting stats: %v\n", CrossMark, err)
		os.Exit(1)
	}

	printRAGStats(stats)
}

func handleRAGList() {
	if err := initRAGEnhanced(); err != nil {
		fmt.Printf("%s Failed to initialize RAG: %v\n", CrossMark, err)
		os.Exit(1)
	}

	sources, err := ragManager.ListSources()
	if err != nil {
		fmt.Printf("%s Error listing sources: %v\n", CrossMark, err)
		os.Exit(1)
	}

	printSourceList(sources)
}

func handleRAGRemove() {
	if len(os.Args) < 4 {
		fmt.Println(colorError("Usage: terminal-ai rag remove <source_path>"))
		os.Exit(1)
	}

	sourcePath := os.Args[3]

	if err := initRAGEnhanced(); err != nil {
		fmt.Printf("%s Failed to initialize RAG: %v\n", CrossMark, err)
		os.Exit(1)
	}

	if err := ragManager.RemoveSource(sourcePath); err != nil {
		fmt.Printf("%s Error removing source: %v\n", CrossMark, err)
		os.Exit(1)
	}

	fmt.Printf("%s Removed: %s\n", TrashIcon, sourcePath)
}

func handleRAGClear() {
	fmt.Printf("%s This will delete ALL RAG data. Continue? [y/N]: ", WarningMark)
	var confirm string
	fmt.Scanln(&confirm)

	if confirm != "y" && confirm != "Y" {
		fmt.Println("Cancelled.")
		os.Exit(0)
	}

	if err := initRAGEnhanced(); err != nil {
		fmt.Printf("%s Failed to initialize RAG: %v\n", CrossMark, err)
		os.Exit(1)
	}

	if err := ragManager.ClearAll(); err != nil {
		fmt.Printf("%s Error clearing: %v\n", CrossMark, err)
		os.Exit(1)
	}

	fmt.Printf("%s All RAG data cleared.\n", CheckMark)
}

func printIndexReport(report *rag.UpdateReport) {
	fmt.Printf("\n%s Index Complete!\n", colorSuccess("✅"))
	fmt.Println(strings.Repeat("═", 50))

	table := NewTable([]string{"Action", "Count"})
	table.SetAlign(1, "right")

	if report.Added > 0 {
		table.AddRow([]string{colorSuccess("➕ Added"), fmt.Sprintf("%d", report.Added)})
	}
	if report.Updated > 0 {
		table.AddRow([]string{colorWarning("🔄 Updated"), fmt.Sprintf("%d", report.Updated)})
	}
	if report.Unchanged > 0 {
		table.AddRow([]string{colorSuccess("✓ Unchanged"), fmt.Sprintf("%d", report.Unchanged)})
	}
	if report.Errors > 0 {
		table.AddRow([]string{colorError("❌ Errors"), fmt.Sprintf("%d", report.Errors)})
	}

	fmt.Print(table.String())
	fmt.Println()

	fmt.Printf("\n%s Total chunks: %d\n", colorInfo("💾"), report.TotalChunks)
	fmt.Printf("%s Time taken: %v\n", colorInfo("🕐"), report.Duration.Round(time.Millisecond))
}

func printAddReport(report *rag.UpdateReport) {
	fmt.Printf("\n%s Update Complete!\n", colorSuccess("✅"))
	fmt.Println(strings.Repeat("═", 50))

	table := NewTable([]string{"Action", "Count"})
	table.SetAlign(1, "right")
	table.AddRow([]string{colorSuccess("➕ Added"), fmt.Sprintf("%d", report.Added)})
	table.AddRow([]string{colorWarning("🔄 Updated"), fmt.Sprintf("%d", report.Updated)})
	table.AddRow([]string{colorSuccess("✓ Unchanged"), fmt.Sprintf("%d", report.Unchanged)})
	table.AddRow([]string{colorError("❌ Errors"), fmt.Sprintf("%d", report.Errors)})

	fmt.Print(table.String())
	fmt.Println()

	fmt.Printf("\n%s Total chunks: %d\n", colorInfo("💾"), report.TotalChunks)
	fmt.Printf("%s Time taken: %v\n", colorInfo("🕐"), report.Duration.Round(time.Millisecond))
}

func printEnhancedSearchResults(results []rag.HybridSearchResult, query string, duration time.Duration) {
	fmt.Printf("\n%s Search: %s\n", SearchIcon, colorBold(query))
	fmt.Println(strings.Repeat("═", 50))

	if len(results) == 0 {
		fmt.Printf("%s No results found.\n", WarningMark)
		return
	}

	for i, result := range results {
		if i > 0 {
			fmt.Println()
		}

		sourceIcon := FileIcon
		if result.SourceType == "web" {
			sourceIcon = WebIcon
		}

		sourceName := result.SourcePath
		if result.SourceType == "web" && result.SourceURL != "" {
			sourceName = result.SourceURL
		}

		if len(sourceName) > 60 {
			sourceName = sourceName[:57] + "..."
		}

		fmt.Printf("%d. %s %s\n", i+1, sourceIcon, sourceName)

		scoreColor := colorCyan
		if result.HybridScore > 0.7 {
			scoreColor = colorSuccess
		} else if result.HybridScore < 0.4 {
			scoreColor = colorWarning
		}

		fmt.Printf("   Score: %s (Vector: %.2f | Keyword: %.2f)\n",
			scoreColor(fmt.Sprintf("%.3f", result.HybridScore)),
			result.VectorScore,
			result.KeywordScore)

		content := result.Content
		if len(content) > 200 {
			content = content[:197] + "..."
		}
		fmt.Printf("   %s\n", content)
	}

	fmt.Printf("\n%s Showing top %d results (%s)\n",
		InfoMark, len(results), duration.Round(time.Millisecond))
}

func printRAGStats(stats *rag.IndexStats) {
	fmt.Printf("\n%s RAG Index Status\n", StatsIcon)
	fmt.Println(strings.Repeat("═", 50))

	fmt.Printf("%s Total Sources: %d\n", ListIcon, stats.TotalSources)

	if stats.Breakdown != nil {
		for sourceType, count := range stats.Breakdown {
			icon := FileIcon
			if sourceType == "web" {
				icon = WebIcon
			}
			fmt.Printf("   %s %s: %d\n", icon, sourceType, count)
		}
	}

	fmt.Println()

	table := NewTable([]string{"Metric", "Value"})
	table.AddRow([]string{colorInfo("💾 Total Chunks"), fmt.Sprintf("%d", stats.TotalChunks)})
	table.AddRow([]string{colorInfo("💾 Total Size"), rag.FormatBytes(stats.TotalSize)})
	table.AddRow([]string{colorInfo("🕐 Last Updated"), stats.LastUpdated.Format("2006-01-02 15:04")})
	fmt.Print(table.String())
}

func printSourceList(sources []rag.SourceInfo) {
	fmt.Printf("\n%s Indexed Sources\n", ListIcon)
	fmt.Println(strings.Repeat("═", 50))

	if len(sources) == 0 {
		fmt.Printf("%s No sources indexed yet.\n", WarningMark)
		return
	}

	for i, source := range sources {
		if i > 0 {
			fmt.Println()
		}

		icon := FileIcon
		if source.SourceType == "web" {
			icon = WebIcon
		}

		table := NewTable([]string{"Property", "Value"})
		table.AddRow([]string{icon + " Source", source.Path})
		table.AddRow([]string{colorInfo("📦 Chunks"), fmt.Sprintf("%d", len(source.ChunkIDs))})
		table.AddRow([]string{colorInfo("📊 Status"), source.Status})
		table.AddRow([]string{colorInfo("🕐 Last Indexed"), source.LastIndexed.Format("2006-01-02 15:04")})
		fmt.Print(table.String())
	}

	fmt.Printf("\n%s Total: %d sources\n", ListIcon, len(sources))
}

func showEnhancedRAGHelp() {
	fmt.Println(colorBold("RAG (Retrieval Augmented Generation) Commands:"))
	fmt.Println()
	fmt.Printf("  %s %s  Index a directory (full reindex)\n", colorCyan("terminal-ai"), colorBold("rag index <dir>"))
	fmt.Printf("  %s %s  Add/update directories (incremental)\n", colorCyan("terminal-ai"), colorBold("rag add <dir> [dir2]..."))
	fmt.Printf("  %s %s  Scrape and index a webpage\n", colorCyan("terminal-ai"), colorBold("rag web <url>"))
	fmt.Printf("  %s %s  Search indexed documents (hybrid search)\n", colorCyan("terminal-ai"), colorBold("rag search <query>"))
	fmt.Printf("  %s %s  Show index statistics\n", colorCyan("terminal-ai"), colorBold("rag status"))
	fmt.Printf("  %s %s  List indexed sources\n", colorCyan("terminal-ai"), colorBold("rag list"))
	fmt.Printf("  %s %s  Remove a source from index\n", colorCyan("terminal-ai"), colorBold("rag remove <path>"))
	fmt.Printf("  %s %s  Clear all RAG data\n", colorCyan("terminal-ai"), colorBold("rag clear"))
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Printf("  terminal-ai rag index ~/Documents\n")
	fmt.Printf("  terminal-ai rag add ~/work ~/personal\n")
	fmt.Printf("  terminal-ai rag web https://example.com\n")
	fmt.Printf("  terminal-ai rag search \"authentication middleware\"\n")
}

func CachedWebTitle(url string) string {
	return ""
}
