package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/mux"
)

type ChatRequest struct {
	Message  string    `json:"message"`
	Provider string    `json:"provider"`
	History  []Message `json:"history"`
}

type ChatResponse struct {
	Response  string `json:"response"`
	Timestamp string `json:"timestamp"`
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
	Directory string `json:"directory"`
}

type RAGSearchRequest struct {
	Query string `json:"query"`
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
	router.HandleFunc("/api/rag/index", authenticate(handleRAGIndex)).Methods("POST")
	router.HandleFunc("/api/rag/search", authenticate(handleRAGSearch)).Methods("POST")
	router.HandleFunc("/api/skills", authenticate(handleListSkills)).Methods("GET")
	router.HandleFunc("/api/users", authenticate(handleListUsers)).Methods("GET")
	router.HandleFunc("/health", handleHealth).Methods("GET")

	corsMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
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
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	token, err := securityMgr.Authenticate(req.Username, req.Password)
	if err != nil {
		http.Error(w, "Authentication failed", http.StatusUnauthorized)
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

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "logged out"})
}

func authenticate(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Authorization header required", http.StatusUnauthorized)
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")
		username, err := securityMgr.ValidateSession(token)
		if err != nil {
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		r.Header.Set("X-Username", username)
		next(w, r)
	}
}

func handleChat(w http.ResponseWriter, r *http.Request) {
	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	providerName := req.Provider
	if providerName == "" {
		providerName = "openrouter"
	}

	provider, exists := providers[providerName]
	if !exists {
		http.Error(w, "Unknown provider", http.StatusBadRequest)
		return
	}

	if provider.APIKey == "" {
		http.Error(w, "API key not configured", http.StatusInternalServerError)
		return
	}

	messages := req.History
	if len(messages) == 0 {
		messages = []Message{{Role: "user", Content: req.Message}}
	} else {
		messages = append(messages, Message{Role: "user", Content: req.Message})
	}

	results := searchRAG(req.Message)
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

	response, err := makeRequest(provider.Endpoint, provider.APIKey, Request{
		Model:    provider.Model,
		Messages: messages,
		Stream:   false,
	}, provider.Name)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if response.Error != nil {
		http.Error(w, response.Error.Message, http.StatusInternalServerError)
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func handleRAGIndex(w http.ResponseWriter, r *http.Request) {
	var req RAGIndexRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	count := 0
	err := filepath.Walk(req.Directory, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".txt" && ext != ".md" && ext != ".json" {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		keywords := extractKeywords(string(content))

		doc := RAGDocument{
			Path:      path,
			Content:   string(content),
			Keywords:  keywords,
			IndexedAt: time.Now().Format(time.RFC3339),
		}

		ragIndex.Documents = append(ragIndex.Documents, doc)
		count++
		return nil
	})

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := saveRAGIndex(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "success",
		"indexed": count,
		"total":   len(ragIndex.Documents),
	})
}

func handleRAGSearch(w http.ResponseWriter, r *http.Request) {
	var req RAGSearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	results := searchRAG(req.Query)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "success",
		"results": results,
		"count":   len(results),
	})
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
