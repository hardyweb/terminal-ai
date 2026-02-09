package main

import (
	"bytes"
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
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type TerminalSession struct {
	ID         string
	WS         *websocket.Conn
	User       string
	Shell      string
	Pty        *os.File
	Cmd        *exec.Cmd
	InputPipe  *io.WriteCloser
	CreatedAt  time.Time
	LastActive time.Time
	mu         sync.Mutex
}

type TerminalMessage struct {
	Type    string `json:"type"`
	Data    string `json:"data,omitempty"`
	Shell   string `json:"shell,omitempty"`
	SessID  string `json:"sessionId,omitempty"`
	Message string `json:"message,omitempty"`
}

var (
	sessionStore = make(map[string]*TerminalSession)
	sessionMu    sync.RWMutex
	maxSessions  = 10
)

var (
	forbiddenCommands = []string{
		"rm -rf /",
		"rm -rf /*",
		"mkfs",
		":(){:|:&}",
		"fork bomb",
		">/dev/null",
		"> /dev/null",
		"dd if=/dev/zero",
	}

	blockedPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(sudo|su)\s+`),
		regexp.MustCompile(`(?i)(chmod|chown)\s+777`),
		regexp.MustCompile(`(?i)(wget|curl)\s+.*\|\s*sh`),
		regexp.MustCompile(`(?i)(nc|netcat)\s+.*-e`),
	}
)

func handleTerminalWebSocket(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "Token required", http.StatusUnauthorized)
		return
	}

	username, err := securityMgr.ValidateSession(token)
	if err != nil {
		http.Error(w, "Invalid token", http.StatusUnauthorized)
		return
	}

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	session := &TerminalSession{
		ID:         uuid.New().String()[:8],
		WS:         ws,
		User:       username,
		Shell:      "/bin/bash",
		CreatedAt:  time.Now(),
		LastActive: time.Now(),
	}

	addSession(session)

	go handleSession(session)
}

func addSession(session *TerminalSession) {
	sessionMu.Lock()
	defer sessionMu.Unlock()

	for len(sessionStore) >= maxSessions {
		var oldest *TerminalSession
		for _, s := range sessionStore {
			if oldest == nil || s.CreatedAt.Before(oldest.CreatedAt) {
				oldest = s
			}
		}
		if oldest != nil {
			removeSession(oldest.ID)
		}
	}

	sessionStore[session.ID] = session
}

func removeSession(id string) {
	sessionMu.Lock()
	defer sessionMu.Unlock()

	if session, exists := sessionStore[id]; exists {
		session.mu.Lock()
		defer session.mu.Unlock()

		if session.Cmd != nil && session.Cmd.Process != nil {
			session.Cmd.Process.Signal(syscall.SIGTERM)
		}
		if session.Pty != nil {
			session.Pty.Close()
		}
		delete(sessionStore, id)
	}
}

func handleSession(session *TerminalSession) {
	defer func() {
		removeSession(session.ID)
		session.WS.Close()
	}()

	go func() {
		for {
			_, p, err := session.WS.ReadMessage()
			if err != nil {
				return
			}

			session.mu.Lock()
			session.LastActive = time.Now()
			session.mu.Unlock()

			var msg TerminalMessage
			if err := json.Unmarshal(p, &msg); err != nil {
				continue
			}

			switch msg.Type {
			case "create":
				createPTYSession(session, msg.Shell)

			case "input":
				if session.InputPipe != nil {
					session.mu.Lock()
					session.LastActive = time.Now()
					session.mu.Unlock()

					_, err := (*session.InputPipe).Write([]byte(msg.Data))
					if err != nil {
						session.WS.WriteJSON(TerminalMessage{
							Type:    "error",
							Message: "Write failed",
						})
					}
				}

			case "resize":
				if session.Pty != nil {
					parts := strings.Split(msg.Data, ",")
					if len(parts) == 2 {
						rows, _ := strconv.Atoi(parts[0])
						cols, _ := strconv.Atoi(parts[1])
						pty.Setsize(session.Pty, &pty.Winsize{
							Rows: uint16(rows),
							Cols: uint16(cols),
						})
					}
				}

			case "kill":
				removeSession(session.ID)
				return
			}
		}
	}()

	buf := make([]byte, 8192)
	for {
		n, err := session.Pty.Read(buf)
		if err != nil {
			break
		}

		data := string(buf[:n])
		if containsForbidden(data) {
			session.WS.WriteJSON(TerminalMessage{
				Type:    "error",
				Message: "Forbidden command detected",
			})
			continue
		}

		if err := session.WS.WriteJSON(TerminalMessage{
			Type: "output",
			Data: data,
		}); err != nil {
			break
		}
	}
}

func createPTYSession(session *TerminalSession, shell string) {
	if shell == "" {
		shell = "/bin/bash"
	}

	var shellPath string
	paths := strings.Split(os.Getenv("PATH"), ":")
	for _, p := range paths {
		shellExec := filepath.Join(p, shell)
		if _, err := os.Stat(shellExec); err == nil {
			shellPath = shellExec
			break
		}
	}

	if shellPath == "" {
		shellPath = shell
	}

	cmd := exec.Command(shellPath, "-i")
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("TERM=xterm-256color"),
		fmt.Sprintf("USER=%s", session.User),
		fmt.Sprintf("HOME=%s", getHomeDir()),
	)

	ptmx, err := pty.Start(cmd)
	if err != nil {
		session.WS.WriteJSON(TerminalMessage{
			Type:    "error",
			Message: fmt.Sprintf("Failed to start shell: %v", err),
		})
		return
	}

	session.Pty = ptmx
	session.Cmd = cmd

	inputPipe, err := cmd.StdinPipe()
	if err != nil {
		session.WS.WriteJSON(TerminalMessage{
			Type:    "error",
			Message: fmt.Sprintf("Failed to get stdin: %v", err),
		})
		return
	}
	session.InputPipe = &inputPipe

	session.WS.WriteJSON(TerminalMessage{
		Type:   "session",
		SessID: session.ID,
	})
}

func containsForbidden(data string) bool {
	data = strings.ToLower(data)
	for _, cmd := range forbiddenCommands {
		if strings.Contains(data, strings.ToLower(cmd)) {
			return true
		}
	}

	for _, pattern := range blockedPatterns {
		if pattern.MatchString(data) {
			return true
		}
	}

	return false
}

func getHomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "/tmp"
	}
	return home
}

func serveTerminalPage(w http.ResponseWriter, r *http.Request) {
	htmlPath := filepath.Join(".", "terminal.html")
	data, err := os.ReadFile(htmlPath)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "Terminal UI not found")
		return
	}

	w.Header().Set("Content-Type", "text/html")
	w.Write(data)
}

func executeSingleCommand(username string, command string) (string, error) {
	if containsForbidden(command) {
		return "", fmt.Errorf("forbidden command detected")
	}

	parts := strings.Fields(command)
	if len(parts) == 0 {
		return "", fmt.Errorf("empty command")
	}

	cmd := exec.Command(parts[0], parts[1:]...)
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("TERM=xterm-256color"),
		fmt.Sprintf("USER=%s", username),
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("command failed: %v", err)
	}

	output := stdout.String()
	if stderr.Len() > 0 {
		output += "\n[stderr]:\n" + stderr.String()
	}

	return output, nil
}

func listTerminalSessions() []map[string]interface{} {
	sessionMu.RLock()
	defer sessionMu.RUnlock()

	var sessions []map[string]interface{}
	for id, session := range sessionStore {
		sessions = append(sessions, map[string]interface{}{
			"id":          id,
			"user":        session.User,
			"shell":       session.Shell,
			"created_at":  session.CreatedAt.Format(time.RFC3339),
			"last_active": session.LastActive.Format(time.RFC3339),
		})
	}

	return sessions
}

func getSessionInfo(id string) (*TerminalSession, bool) {
	sessionMu.RLock()
	defer sessionMu.RUnlock()

	session, exists := sessionStore[id]
	return session, exists
}
