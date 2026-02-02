package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/mux"
)

type ChatRequest struct {
	Message   string    `json:"message"`
	Provider  string    `json:"provider"`
	History   []Message `json:"history"`
	SessionID string    `json:"session_id,omitempty"`
}

type ChatResponse struct {
	Response  string `json:"response"`
	Timestamp string `json:"timestamp"`
	SessionID string `json:"session_id,omitempty"`
}

type HistoryCreateRequest struct {
	Message  string `json:"message"`
	Provider string `json:"provider"`
}

type HistoryUpdateRequest struct {
	Message  string `json:"message"`
	Provider string `json:"provider"`
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Token    string `json:"token"`
	Username string `json:"username"`
}

type RAGIndexRequest struct {
	Directory  string `json:"directory"`
	Visibility string `json:"visibility"`
}

type RAGSearchRequest struct {
	Query      string `json:"query"`
	Visibility string `json:"visibility"`
}

// Helper function to send JSON error responses
func sendJSONError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// Helper function to send JSON success responses
func sendJSONResponse(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(data)
}

func startWebServer() {
	router := mux.NewRouter()

	port := os.Getenv("WEB_PORT")
	if port == "" {
		port = "8080"
	}

	host := os.Getenv("WEB_HOST")
	if host == "" {
		host = "localhost"
	}

	router.HandleFunc("/", serveWebUI).Methods("GET")
	router.HandleFunc("/api/login", handleLogin).Methods("POST")
	router.HandleFunc("/api/logout", handleLogout).Methods("POST")
	router.HandleFunc("/api/chat", authenticate(handleChat)).Methods("POST")
	router.HandleFunc("/api/chat/stream", authenticate(handleChatStream)).Methods("POST")
	router.HandleFunc("/api/chat/public", handlePublicChat).Methods("POST")
	router.HandleFunc("/api/rag/index", authenticate(handleRAGIndex)).Methods("POST")
	router.HandleFunc("/api/rag/search", authenticate(handleRAGSearch)).Methods("POST")
	router.HandleFunc("/api/rag/search/public", handlePublicRAGSearch).Methods("POST")
	router.HandleFunc("/api/skills", authenticate(handleListSkills)).Methods("GET")
	router.HandleFunc("/api/users", authenticate(handleListUsers)).Methods("GET")
	router.HandleFunc("/api/history", authenticate(handleListHistory)).Methods("GET")
	router.HandleFunc("/api/history", authenticate(handleCreateSession)).Methods("POST")
	router.HandleFunc("/api/history/{id}", authenticate(handleGetSession)).Methods("GET")
	router.HandleFunc("/api/history/{id}", authenticate(handleUpdateSession)).Methods("PUT")
	router.HandleFunc("/api/history/{id}", authenticate(handleDeleteSession)).Methods("DELETE")
	router.HandleFunc("/api/providers", authenticate(handleListProviders)).Methods("GET")
	router.HandleFunc("/api/providers/{name}", authenticate(handleGetProvider)).Methods("GET")
	router.HandleFunc("/api/providers/{name}/enable", authenticate(handleEnableProvider)).Methods("POST")
	router.HandleFunc("/api/providers/{name}/disable", authenticate(handleDisableProvider)).Methods("POST")
	router.HandleFunc("/api/providers/{name}/priority", authenticate(handleSetProviderPriority)).Methods("PUT")
	router.HandleFunc("/api/providers/{name}/default", authenticate(handleSetDefaultProvider)).Methods("POST")
	router.HandleFunc("/api/providers/{name}/test", authenticate(handleTestProvider)).Methods("POST")
	router.HandleFunc("/api/providers", authenticate(handleAddProvider)).Methods("POST")
	router.HandleFunc("/api/providers/{name}", authenticate(handleDeleteProvider)).Methods("DELETE")
	// OpenRouter BYOK endpoints
	router.HandleFunc("/api/providers/openrouter/byok", authenticate(handleGetBYOKConfig)).Methods("GET")
	router.HandleFunc("/api/providers/openrouter/byok", authenticate(handleUpdateBYOKConfig)).Methods("PUT")
	router.HandleFunc("/api/providers/openrouter/byok/test", authenticate(handleTestBYOK)).Methods("POST")
	router.HandleFunc("/health", handleHealth).Methods("GET")

	corsMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}

			next.ServeHTTP(w, r)
		})
	}

	fmt.Printf("🚀 Web server starting on http://%s:%s\n", host, port)
	log.Fatal(http.ListenAndServe(host+":"+port, corsMiddleware(router)))
}

func serveWebUI(w http.ResponseWriter, r *http.Request) {
	htmlPath := filepath.Join(".", "ui.html")
	data, err := os.ReadFile(htmlPath)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "Web UI not found")
		return
	}

	w.Header().Set("Content-Type", "text/html")
	w.Write(data)
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendJSONError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	token, err := securityMgr.Authenticate(req.Username, req.Password)
	if err != nil {
		sendJSONError(w, http.StatusUnauthorized, "Authentication failed")
		return
	}

	response := LoginResponse{
		Token:    token,
		Username: req.Username,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
		token := strings.TrimPrefix(authHeader, "Bearer ")
		securityMgr.Logout(token)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "logged out"})
}

func authenticate(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			sendJSONError(w, http.StatusUnauthorized, "Authorization header required")
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")
		username, err := securityMgr.ValidateSession(token)
		if err != nil {
			sendJSONError(w, http.StatusUnauthorized, "Invalid token")
			return
		}

		r.Header.Set("X-Username", username)
		next(w, r)
	}
}

func handleChat(w http.ResponseWriter, r *http.Request) {
	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendJSONError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	providerName := req.Provider
	if providerName == "" {
		providerName = providerConfig.DefaultProvider
	}

	provider, exists := providers[providerName]
	if !exists {
		sendJSONError(w, http.StatusBadRequest, "Unknown provider")
		return
	}

	if provider.APIKey == "" {
		sendJSONError(w, http.StatusInternalServerError, "API key not configured")
		return
	}

	messages := req.History
	if len(messages) == 0 {
		messages = []Message{{Role: "user", Content: req.Message}}
	} else {
		messages = append(messages, Message{Role: "user", Content: req.Message})
	}

	username := r.Header.Get("X-Username")
	results := searchRAGWithFilters(req.Message, username, "")
	if len(results) > 0 {
		context := "\n\nRelevant documents:\n"
		for _, doc := range results {
			contentLen := len(doc.Content)
			if contentLen > 200 {
				contentLen = 200
			}
			context += fmt.Sprintf("- %s: %s\n", doc.Path, doc.Content[:contentLen])
		}
		messages[len(messages)-1].Content += context
	}

	var response *Response
	var actualProvider string
	var err error

	if providerConfig.FallbackEnabled {
		response, actualProvider, err = makeRequestWithFallback(
			provider.Endpoint, provider.APIKey, Request{
				Model:    provider.Model,
				Messages: messages,
				Stream:   false,
			}, providerName,
		)
	} else {
		response, err = makeRequest(provider.Endpoint, provider.APIKey, Request{
			Model:    provider.Model,
			Messages: messages,
			Stream:   false,
		}, provider.Name)
		actualProvider = providerName
	}

	if err != nil {
		sendJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if response.Error != nil {
		sendJSONError(w, http.StatusInternalServerError, response.Error.Message)
		return
	}

	var content string
	if len(response.Choices) > 0 {
		content = response.Choices[0].Message.Content
	} else {
		content = "No response generated"
	}

	resp := ChatResponse{
		Response:  content,
		Timestamp: time.Now().Format(time.RFC3339),
	}

	if actualProvider != req.Provider && req.Provider != "" {
		resp.Response = fmt.Sprintf("[Provider: %s] %s", actualProvider, content)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func handleChatStream(w http.ResponseWriter, r *http.Request) {
	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		fmt.Fprintf(w, "data: {\"error\": \"Invalid request\"}\n\n")
		return
	}

	providerName := req.Provider
	if providerName == "" {
		providerName = providerConfig.DefaultProvider
	}

	provider, exists := providers[providerName]
	if !exists {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		fmt.Fprintf(w, "data: {\"error\": \"Unknown provider\"}\n\n")
		return
	}

	if provider.APIKey == "" {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		fmt.Fprintf(w, "data: {\"error\": \"API key not configured\"}\n\n")
		return
	}

	messages := req.History
	if len(messages) == 0 {
		messages = []Message{{Role: "user", Content: req.Message}}
	} else {
		messages = append(messages, Message{Role: "user", Content: req.Message})
	}

	username := r.Header.Get("X-Username")
	results := searchRAGWithFilters(req.Message, username, "")
	if len(results) > 0 {
		context := "\n\nRelevant documents:\n"
		for _, doc := range results {
			contentLen := len(doc.Content)
			if contentLen > 200 {
				contentLen = 200
			}
			context += fmt.Sprintf("- %s: %s\n", doc.Path, doc.Content[:contentLen])
		}
		messages[len(messages)-1].Content += context
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Get flusher
	flusher, ok := w.(http.Flusher)
	if !ok {
		fmt.Fprintf(w, "data: {\"error\": \"Streaming not supported\"}\n\n")
		return
	}

	// Build request
	aiReq := Request{
		Model:    provider.Model,
		Messages: messages,
		Stream:   true,
	}

	var reqBody []byte
	var err error

	// Check if OpenRouter with BYOK enabled
	if providerName == "openrouter" {
		if config, exists := providerConfig.Providers["openrouter"]; exists && config.BYOKConfig != nil && config.BYOKConfig.Enabled {
			openRouterReq := OpenRouterRequest{
				Model:    aiReq.Model,
				Messages: aiReq.Messages,
				Stream:   true,
				Provider: &OpenRouterProvider{
					AllowFallbacks: config.BYOKConfig.AllowFallbackToShared,
					Order:          config.BYOKConfig.ProviderOrder,
				},
			}
			reqBody, err = json.Marshal(openRouterReq)
		} else {
			reqBody, err = json.Marshal(aiReq)
		}
	} else {
		reqBody, err = json.Marshal(aiReq)
	}

	if err != nil {
		fmt.Fprintf(w, "data: {\"error\": \"Failed to marshal request\"}\n\n")
		flusher.Flush()
		return
	}

	// Make HTTP request with extended timeout for streaming
	// Using 300 seconds (5 minutes) to handle long articles
	client := &http.Client{Timeout: 300 * time.Second}
	httpReq, err := http.NewRequest("POST", provider.Endpoint, strings.NewReader(string(reqBody)))
	if err != nil {
		fmt.Fprintf(w, "data: {\"error\": \"%s\"}\n\n", err.Error())
		flusher.Flush()
		return
	}

	httpReq.Header.Set("Content-Type", "application/json")

	if providerName == "openrouter" {
		httpReq.Header.Set("Authorization", "Bearer "+provider.APIKey)
		httpReq.Header.Set("HTTP-Referer", "https://terminal-ai.local")
		httpReq.Header.Set("X-Title", "Terminal AI CLI")
	} else if providerName == "gemini" {
		httpReq.Header.Set("x-goog-api-key", provider.APIKey)
	} else {
		httpReq.Header.Set("Authorization", "Bearer "+provider.APIKey)
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		fmt.Fprintf(w, "data: {\"error\": \"%s\"}\n\n", err.Error())
		flusher.Flush()
		return
	}
	defer resp.Body.Close()

	// Stream response
	reader := bufio.NewReader(resp.Body)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			fmt.Fprintf(w, "data: {\"error\": \"%s\"}\n\n", err.Error())
			flusher.Flush()
			return
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Check for SSE data prefix
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")

		// Check for stream end
		if data == "[DONE]" {
			fmt.Fprintf(w, "data: [DONE]\n\n")
			flusher.Flush()
			break
		}

		// Parse the streaming response
		var streamResp StreamingResponse
		if err := json.Unmarshal([]byte(data), &streamResp); err != nil {
			// Some providers might send different formats, skip unparseable lines
			continue
		}

		// Check for API errors in stream
		if streamResp.Error != nil {
			fmt.Fprintf(w, "data: {\"error\": \"%s\"}\n\n", streamResp.Error.Message)
			flusher.Flush()
			return
		}

		// Extract content
		if len(streamResp.Choices) > 0 {
			content := streamResp.Choices[0].Delta.Content
			if content != "" {
				// Escape quotes in content for JSON
				content = strings.ReplaceAll(content, "\\", "\\\\")
				content = strings.ReplaceAll(content, "\"", "\\\"")
				fmt.Fprintf(w, "data: {\"content\": \"%s\"}\n\n", content)
				flusher.Flush()
			}
		}
	}

	// Send final DONE message
	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}

func handleListSkills(w http.ResponseWriter, r *http.Request) {
	homeDir, _ := os.UserHomeDir()
	skillsDir := filepath.Join(homeDir, configDir, "skills")

	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		json.NewEncoder(w).Encode([]Skill{})
		return
	}

	var skills []Skill
	for _, entry := range entries {
		if entry.IsDir() {
			skillFile := filepath.Join(skillsDir, entry.Name(), "skill.json")
			data, err := os.ReadFile(skillFile)
			if err == nil {
				var skill Skill
				json.Unmarshal(data, &skill)
				skills = append(skills, skill)
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(skills)
}

func handleListUsers(w http.ResponseWriter, r *http.Request) {
	var users []User
	for _, user := range securityMgr.users {
		users = append(users, user)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(users)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
		"time":   time.Now().Format(time.RFC3339),
	})
}

func handleListHistory(w http.ResponseWriter, r *http.Request) {
	username := r.Header.Get("X-Username")
	sessions := listSessions()

	var userSessions []ChatSession
	for _, session := range sessions {
		if session.User == username {
			userSessions = append(userSessions, session)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(userSessions)
}

func handleGetSession(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sessionID := vars["id"]
	username := r.Header.Get("X-Username")

	session, err := getSession(sessionID)
	if err != nil {
		sendJSONError(w, http.StatusNotFound, "Session not found")
		return
	}

	if session.User != username {
		sendJSONError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(session)
}

func handleCreateSession(w http.ResponseWriter, r *http.Request) {
	var req HistoryCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendJSONError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	username := r.Header.Get("X-Username")
	providerName := req.Provider
	if providerName == "" {
		providerName = providerConfig.DefaultProvider
	}

	provider, exists := providers[providerName]
	if !exists {
		sendJSONError(w, http.StatusBadRequest, "Unknown provider")
		return
	}

	if provider.APIKey == "" {
		sendJSONError(w, http.StatusInternalServerError, "API key not configured")
		return
	}

	session := createSession(req.Message, providerName, username)
	updateSession(session.ID, "user", req.Message)

	messages := []Message{{Role: "user", Content: req.Message}}

	results := searchRAGWithFilters(req.Message, username, "")
	if len(results) > 0 {
		context := "\n\nRelevant documents:\n"
		for _, doc := range results {
			contentLen := len(doc.Content)
			if contentLen > 200 {
				contentLen = 200
			}
			context += fmt.Sprintf("- %s: %s\n", doc.Path, doc.Content[:contentLen])
		}
		messages[0].Content += context
	}

	var response *Response
	var aiErr error

	if providerConfig.FallbackEnabled {
		response, _, aiErr = makeRequestWithFallback(
			provider.Endpoint, provider.APIKey, Request{
				Model:    provider.Model,
				Messages: messages,
				Stream:   false,
			}, providerName,
		)
	} else {
		response, aiErr = makeRequest(provider.Endpoint, provider.APIKey, Request{
			Model:    provider.Model,
			Messages: messages,
			Stream:   false,
		}, provider.Name)
	}

	if aiErr != nil {
		sendJSONError(w, http.StatusInternalServerError, aiErr.Error())
		return
	}

	if response.Error != nil {
		sendJSONError(w, http.StatusInternalServerError, response.Error.Message)
		return
	}

	var content string
	if len(response.Choices) > 0 {
		content = response.Choices[0].Message.Content
		updateSession(session.ID, "assistant", content)
	} else {
		content = "No response generated"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"session_id": session.ID,
		"response":   content,
	})
}

func handleUpdateSession(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sessionID := vars["id"]
	username := r.Header.Get("X-Username")

	session, sessionErr := getSession(sessionID)
	if sessionErr != nil {
		sendJSONError(w, http.StatusNotFound, "Session not found")
		return
	}

	if session.User != username {
		sendJSONError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req HistoryUpdateRequest
	if decodeErr := json.NewDecoder(r.Body).Decode(&req); decodeErr != nil {
		sendJSONError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	providerName := req.Provider
	if providerName == "" {
		providerName = session.Provider
	}

	provider, exists := providers[providerName]
	if !exists {
		sendJSONError(w, http.StatusBadRequest, "Unknown provider")
		return
	}

	if provider.APIKey == "" {
		sendJSONError(w, http.StatusInternalServerError, "API key not configured")
		return
	}

	updateSession(sessionID, "user", req.Message)

	messages := []Message{{Role: "user", Content: req.Message}}
	for _, msg := range session.Messages {
		if msg.Role == "user" || msg.Role == "assistant" {
			messages = append(messages, Message{Role: msg.Role, Content: msg.Content})
		}
	}

	results := searchRAGWithFilters(req.Message, username, "")
	if len(results) > 0 {
		context := "\n\nRelevant documents:\n"
		for _, doc := range results {
			contentLen := len(doc.Content)
			if contentLen > 200 {
				contentLen = 200
			}
			context += fmt.Sprintf("- %s: %s\n", doc.Path, doc.Content[:contentLen])
		}
		messages[len(messages)-1].Content += context
	}

	var response *Response
	var aiErr error

	if providerConfig.FallbackEnabled {
		response, _, aiErr = makeRequestWithFallback(
			provider.Endpoint, provider.APIKey, Request{
				Model:    provider.Model,
				Messages: messages,
				Stream:   false,
			}, providerName,
		)
	} else {
		response, aiErr = makeRequest(provider.Endpoint, provider.APIKey, Request{
			Model:    provider.Model,
			Messages: messages,
			Stream:   false,
		}, provider.Name)
	}

	if aiErr != nil {
		sendJSONError(w, http.StatusInternalServerError, aiErr.Error())
		return
	}

	if response.Error != nil {
		sendJSONError(w, http.StatusInternalServerError, response.Error.Message)
		return
	}

	var content string
	if len(response.Choices) > 0 {
		content = response.Choices[0].Message.Content
		updateSession(sessionID, "assistant", content)
	} else {
		content = "No response generated"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"response":  content,
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

func handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sessionID := vars["id"]
	username := r.Header.Get("X-Username")

	session, err := getSession(sessionID)
	if err != nil {
		sendJSONError(w, http.StatusNotFound, "Session not found")
		return
	}

	if session.User != username {
		sendJSONError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	if err := deleteSession(sessionID); err != nil {
		sendJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
}

// OpenRouter BYOK Handlers

func handleGetBYOKConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	openrouterConfig, exists := providerConfig.Providers["openrouter"]
	if !exists {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"enabled":                  false,
			"provider_order":           []string{},
			"allow_fallback_to_shared": true,
			"models":                   map[string]string{},
		})
		return
	}

	if openrouterConfig.BYOKConfig == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"enabled":                  false,
			"provider_order":           []string{},
			"allow_fallback_to_shared": true,
			"models":                   map[string]string{},
		})
		return
	}

	json.NewEncoder(w).Encode(openrouterConfig.BYOKConfig)
}

func handleUpdateBYOKConfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled               bool              `json:"enabled"`
		ProviderOrder         []string          `json:"provider_order"`
		AllowFallbackToShared bool              `json:"allow_fallback_to_shared"`
		Models                map[string]string `json:"models"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	openrouterConfig, exists := providerConfig.Providers["openrouter"]
	if !exists {
		sendJSONError(w, http.StatusNotFound, "OpenRouter provider not found")
		return
	}

	openrouterConfig.BYOKConfig = &OpenRouterBYOKConfig{
		Enabled:               req.Enabled,
		ProviderOrder:         req.ProviderOrder,
		AllowFallbackToShared: req.AllowFallbackToShared,
		Models:                req.Models,
	}

	providerConfig.Providers["openrouter"] = openrouterConfig

	if err := saveProviderConfig(); err != nil {
		sendJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "updated"})
}

func handleTestBYOK(w http.ResponseWriter, r *http.Request) {
	openrouterConfig, exists := providerConfig.Providers["openrouter"]
	if !exists || openrouterConfig.BYOKConfig == nil || !openrouterConfig.BYOKConfig.Enabled {
		sendJSONError(w, http.StatusBadRequest, "BYOK not enabled")
		return
	}

	type TestResult struct {
		Provider string `json:"provider"`
		Success  bool   `json:"success"`
		Message  string `json:"message"`
	}

	results := []TestResult{}
	fallbackUsed := false

	// Get OpenRouter provider
	provider, exists := providers["openrouter"]
	if !exists || provider.APIKey == "" {
		sendJSONError(w, http.StatusBadRequest, "OpenRouter not configured")
		return
	}

	// Get the first provider's model to test with
	var testModel string
	if len(openrouterConfig.BYOKConfig.ProviderOrder) > 0 {
		firstProvider := openrouterConfig.BYOKConfig.ProviderOrder[0]
		modelKey := normalizeProviderKey(firstProvider)
		testModel = openrouterConfig.BYOKConfig.Models[modelKey]
	}

	// Fallback to default model if no BYOK model configured
	if testModel == "" {
		testModel = provider.Model
	}

	// Test with a simple request
	req := Request{
		Model: testModel,
		Messages: []Message{
			{Role: "user", Content: "Hello! Say 'BYOK test successful' if you receive this."},
		},
		Stream: false,
	}

	// Build OpenRouter request with BYOK
	openRouterReq := OpenRouterRequest{
		Model:    req.Model,
		Messages: req.Messages,
		Stream:   req.Stream,
		Provider: &OpenRouterProvider{
			AllowFallbacks: openrouterConfig.BYOKConfig.AllowFallbackToShared,
			Order:          openrouterConfig.BYOKConfig.ProviderOrder,
		},
	}

	reqBody, _ := json.Marshal(openRouterReq)

	client := &http.Client{Timeout: 30 * time.Second}
	httpReq, _ := http.NewRequest("POST", provider.Endpoint, strings.NewReader(string(reqBody)))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+provider.APIKey)
	httpReq.Header.Set("HTTP-Referer", "https://terminal-ai.local")
	httpReq.Header.Set("X-Title", "Terminal AI CLI")

	resp, err := client.Do(httpReq)
	if err != nil {
		results = append(results, TestResult{
			Provider: "OpenRouter",
			Success:  false,
			Message:  err.Error(),
		})
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"results":       results,
			"fallback_used": false,
		})
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var response Response
	json.Unmarshal(body, &response)

	if response.Error != nil {
		results = append(results, TestResult{
			Provider: "OpenRouter",
			Success:  false,
			Message:  response.Error.Message,
		})
	} else if len(response.Choices) > 0 {
		// Check if fallback was used based on response
		fallbackUsed = strings.Contains(response.Choices[0].Message.Content, "OpenRouter") ||
			!strings.Contains(response.Choices[0].Message.Content, "BYOK")

		for _, byokProvider := range openrouterConfig.BYOKConfig.ProviderOrder {
			results = append(results, TestResult{
				Provider: byokProvider,
				Success:  true,
				Message:  "BYOK configured and responding",
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"results":       results,
		"fallback_used": fallbackUsed,
	})
}

// normalizeProviderKey converts provider name to a safe key for map storage
func normalizeProviderKey(name string) string {
	// Convert to lowercase and replace spaces/special chars with underscores
	key := strings.ToLower(name)
	key = strings.ReplaceAll(key, " ", "_")
	key = strings.ReplaceAll(key, "-", "_")
	// Remove any remaining non-alphanumeric characters
	var result strings.Builder
	for _, r := range key {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			result.WriteRune(r)
		}
	}
	return result.String()
}

func handleRAGIndex(w http.ResponseWriter, r *http.Request) {
	var req RAGIndexRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendJSONError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	if req.Directory == "" {
		sendJSONError(w, http.StatusBadRequest, "Directory path required")
		return
	}

	// Get username from context (set by authenticate middleware)
	username := r.Header.Get("X-Username")
	if req.Visibility == "" {
		req.Visibility = "private"
	}

	indexDirectoryWithOwner(req.Directory, username, req.Visibility)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "success",
		"directory": req.Directory,
		"message":   "Directory indexed successfully",
	})
}

func handleRAGSearch(w http.ResponseWriter, r *http.Request) {
	var req RAGSearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendJSONError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	// Get username from context
	username := r.Header.Get("X-Username")

	results := searchRAGWithFilters(req.Query, username, req.Visibility)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "success",
		"results": results,
		"count":   len(results),
	})
}

func handlePublicRAGSearch(w http.ResponseWriter, r *http.Request) {
	var req RAGSearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendJSONError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	results := searchRAGWithFilters(req.Query, "", "public")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "success",
		"results": results,
		"count":   len(results),
	})
}

func handlePublicChat(w http.ResponseWriter, r *http.Request) {
	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendJSONError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	providerName := req.Provider
	if providerName == "" {
		providerName = providerConfig.DefaultProvider
	}

	provider, exists := providers[providerName]
	if !exists {
		sendJSONError(w, http.StatusBadRequest, "Unknown provider")
		return
	}

	if provider.APIKey == "" {
		sendJSONError(w, http.StatusInternalServerError, "API key not configured")
		return
	}

	messages := req.History
	if len(messages) == 0 {
		messages = []Message{{Role: "user", Content: req.Message}}
	} else {
		messages = append(messages, Message{Role: "user", Content: req.Message})
	}

	results := searchRAGWithFilters(req.Message, "", "public")
	if len(results) > 0 {
		context := "\n\nRelevant documents:\n"
		for _, doc := range results {
			contentLen := len(doc.Content)
			if contentLen > 200 {
				contentLen = 200
			}
			context += fmt.Sprintf("- %s: %s\n", doc.Path, doc.Content[:contentLen])
		}
		messages[len(messages)-1].Content += context
	}

	var response *Response
	var actualProvider string
	var err error

	if providerConfig.FallbackEnabled {
		response, actualProvider, err = makeRequestWithFallback(
			provider.Endpoint, provider.APIKey, Request{
				Model:    provider.Model,
				Messages: messages,
				Stream:   false,
			}, providerName,
		)
	} else {
		response, err = makeRequest(provider.Endpoint, provider.APIKey, Request{
			Model:    provider.Model,
			Messages: messages,
			Stream:   false,
		}, provider.Name)
		actualProvider = providerName
	}

	if err != nil {
		sendJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if response.Error != nil {
		sendJSONError(w, http.StatusInternalServerError, response.Error.Message)
		return
	}

	var content string
	if len(response.Choices) > 0 {
		content = response.Choices[0].Message.Content
	} else {
		content = "No response generated"
	}

	resp := ChatResponse{
		Response:  content,
		Timestamp: time.Now().Format(time.RFC3339),
	}

	if actualProvider != req.Provider && req.Provider != "" {
		resp.Response = fmt.Sprintf("[Provider: %s] %s", actualProvider, content)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

type ProviderInfo struct {
	Name        string `json:"name"`
	Priority    int    `json:"priority"`
	Enabled     bool   `json:"enabled"`
	MaxRetries  int    `json:"max_retries"`
	Endpoint    string `json:"endpoint"`
	Model       string `json:"model"`
	BYOK        bool   `json:"byok"`
	IsDefault   bool   `json:"is_default"`
	APIKey      string `json:"api_key"`
	Description string `json:"description"`
}

type AddProviderRequest struct {
	Name     string `json:"name"`
	Priority int    `json:"priority"`
	Endpoint string `json:"endpoint"`
	Model    string `json:"model"`
	APIKey   string `json:"api_key"`
}

type SetPriorityRequest struct {
	Priority int `json:"priority"`
}

func handleListProviders(w http.ResponseWriter, r *http.Request) {
	var providerList []ProviderInfo

	orderedProviders := getOrderedProviders()

	for _, providerName := range orderedProviders {
		config := providerConfig.Providers[providerName]
		provider := providers[providerName]

		info := ProviderInfo{
			Name:        providerName,
			Priority:    config.Priority,
			Enabled:     config.Enabled,
			MaxRetries:  config.MaxRetries,
			Endpoint:    provider.Endpoint,
			Model:       provider.Model,
			BYOK:        config.BYOK,
			IsDefault:   providerName == providerConfig.DefaultProvider,
			Description: config.Description,
		}

		if provider.APIKey != "" {
			info.APIKey = "***configured***"
		}

		providerList = append(providerList, info)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(providerList)
}

func handleGetProvider(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	providerName := vars["name"]

	config, exists := providerConfig.Providers[providerName]
	if !exists {
		sendJSONError(w, http.StatusNotFound, "Provider not found")
		return
	}

	provider := providers[providerName]

	info := ProviderInfo{
		Name:        providerName,
		Priority:    config.Priority,
		Enabled:     config.Enabled,
		MaxRetries:  config.MaxRetries,
		Endpoint:    provider.Endpoint,
		Model:       provider.Model,
		BYOK:        config.BYOK,
		IsDefault:   providerName == providerConfig.DefaultProvider,
		Description: config.Description,
	}

	if provider.APIKey != "" {
		info.APIKey = "***configured***"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

func handleEnableProvider(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	providerName := vars["name"]

	config, exists := providerConfig.Providers[providerName]
	if !exists {
		sendJSONError(w, http.StatusNotFound, "Provider not found")
		return
	}

	config.Enabled = true
	providerConfig.Providers[providerName] = config

	if err := saveProviderConfig(); err != nil {
		sendJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "enabled"})
}

func handleDisableProvider(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	providerName := vars["name"]

	config, exists := providerConfig.Providers[providerName]
	if !exists {
		sendJSONError(w, http.StatusNotFound, "Provider not found")
		return
	}

	config.Enabled = false
	providerConfig.Providers[providerName] = config

	if err := saveProviderConfig(); err != nil {
		sendJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "disabled"})
}

func handleSetProviderPriority(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	providerName := vars["name"]

	var req SetPriorityRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendJSONError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	config, exists := providerConfig.Providers[providerName]
	if !exists {
		sendJSONError(w, http.StatusNotFound, "Provider not found")
		return
	}

	config.Priority = req.Priority
	providerConfig.Providers[providerName] = config

	if err := saveProviderConfig(); err != nil {
		sendJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"status": "updated", "priority": req.Priority})
}

func handleSetDefaultProvider(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	providerName := vars["name"]

	_, exists := providerConfig.Providers[providerName]
	if !exists {
		sendJSONError(w, http.StatusNotFound, "Provider not found")
		return
	}

	providerConfig.DefaultProvider = providerName

	if err := saveProviderConfig(); err != nil {
		sendJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"default_provider": providerName})
}

func handleTestProvider(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	providerName := vars["name"]

	config, exists := providerConfig.Providers[providerName]
	if !exists {
		sendJSONError(w, http.StatusNotFound, "Provider not found")
		return
	}

	if !config.Enabled {
		sendJSONError(w, http.StatusBadRequest, "Provider is disabled")
		return
	}

	provider, exists := providers[providerName]
	if !exists {
		sendJSONError(w, http.StatusNotFound, "Provider not initialized")
		return
	}

	if provider.APIKey == "" {
		sendJSONError(w, http.StatusInternalServerError, "API key not configured")
		return
	}

	req := Request{
		Model: provider.Model,
		Messages: []Message{
			{Role: "user", Content: "Hello! Say 'Test successful' if you receive this."},
		},
		Stream: false,
	}

	response, err := makeRequest(provider.Endpoint, provider.APIKey, req, provider.Name)

	if err != nil {
		sendJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if response.Error != nil {
		sendJSONError(w, http.StatusInternalServerError, response.Error.Message)
		return
	}

	var content string
	if len(response.Choices) > 0 {
		content = response.Choices[0].Message.Content
		if len(content) > 100 {
			content = content[:100]
		}
	} else {
		content = "No response received"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success", "response": content})
}

func handleAddProvider(w http.ResponseWriter, r *http.Request) {
	var req AddProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendJSONError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	if req.Name == "" {
		sendJSONError(w, http.StatusBadRequest, "Provider name is required")
		return
	}

	if _, exists := providerConfig.Providers[req.Name]; exists {
		sendJSONError(w, http.StatusConflict, "Provider already exists")
		return
	}

	config := AIProviderConfig{
		Priority:    req.Priority,
		Enabled:     true,
		MaxRetries:  2,
		EnvKey:      strings.ToUpper(req.Name) + "_API_KEY",
		EndpointKey: strings.ToUpper(req.Name) + "_ENDPOINT",
		ModelKey:    strings.ToUpper(req.Name) + "_MODEL",
		BYOK:        true,
		Description: "Custom BYOK provider",
		GopassKey:   "terminal-ai/" + req.Name + "_api_key",
	}

	providerConfig.Providers[req.Name] = config

	providers[req.Name] = AIProvider{
		Name:     req.Name,
		APIKey:   req.APIKey,
		Endpoint: req.Endpoint,
		Model:    req.Model,
	}

	if err := saveProviderConfig(); err != nil {
		sendJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"status": "created", "name": req.Name})
}

func handleDeleteProvider(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	providerName := vars["name"]

	if providerName == providerConfig.DefaultProvider {
		sendJSONError(w, http.StatusBadRequest, "Cannot delete default provider")
		return
	}

	if _, exists := providerConfig.Providers[providerName]; !exists {
		sendJSONError(w, http.StatusNotFound, "Provider not found")
		return
	}

	delete(providerConfig.Providers, providerName)
	delete(providers, providerName)

	if err := saveProviderConfig(); err != nil {
		sendJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
}
