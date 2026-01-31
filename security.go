package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

const (
	configDir = ".config/terminal-ai"
)

type User struct {
	Username string    `json:"username"`
	Password string    `json:"password_hash"`
	Salt     string    `json:"salt"`
	Created  time.Time `json:"created"`
	Role     string    `json:"role"`
}

type Session struct {
	Token     string    `json:"token"`
	Username  string    `json:"username"`
	Created   time.Time `json:"created"`
	ExpiresAt time.Time `json:"expires_at"`
}

type SecurityManager struct {
	encryptionKey []byte
	sessions      map[string]Session
	users         map[string]User
}

var securityMgr *SecurityManager

func initSecurityManager() *SecurityManager {
	homeDir, _ := os.UserHomeDir()
	keyFile := filepath.Join(homeDir, configDir, ".encryption_key")

	var key []byte
	if data, err := os.ReadFile(keyFile); err == nil {
		key = data
	} else {
		key = make([]byte, 32)
		rand.Read(key)
		os.WriteFile(keyFile, key, 0600)
	}

	mgr := &SecurityManager{
		encryptionKey: key,
		sessions:      make(map[string]Session),
		users:         make(map[string]User),
	}

	mgr.loadUsers()
	return mgr
}

func (sm *SecurityManager) hashPassword(password, salt string) string {
	hash := sha256.Sum256([]byte(password + salt))
	return fmt.Sprintf("%x", hash)
}

func (sm *SecurityManager) generateSalt() string {
	salt := make([]byte, 16)
	rand.Read(salt)
	return base64.StdEncoding.EncodeToString(salt)
}

func (sm *SecurityManager) generateToken() string {
	token := make([]byte, 32)
	rand.Read(token)
	return base64.StdEncoding.EncodeToString(token)
}

func (sm *SecurityManager) encrypt(text string) (string, error) {
	block, err := aes.NewCipher(sm.encryptionKey)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(text), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func (sm *SecurityManager) decrypt(ciphertext string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(sm.encryptionKey)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	nonce, ciphertextData := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertextData, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

func (sm *SecurityManager) loadUsers() error {
	homeDir, _ := os.UserHomeDir()
	usersFile := filepath.Join(homeDir, configDir, "users", "users.json")

	data, err := os.ReadFile(usersFile)
	if err != nil {
		return nil
	}

	var users []User
	if err := json.Unmarshal(data, &users); err != nil {
		return err
	}

	for _, user := range users {
		sm.users[user.Username] = user
	}

	return nil
}

func (sm *SecurityManager) saveUsers() error {
	homeDir, _ := os.UserHomeDir()
	usersFile := filepath.Join(homeDir, configDir, "users", "users.json")

	var users []User
	for _, user := range sm.users {
		users = append(users, user)
	}

	data, err := json.MarshalIndent(users, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(usersFile, data, 0600)
}

func (sm *SecurityManager) CreateUser(username, password, role string) error {
	if _, exists := sm.users[username]; exists {
		return errors.New("user already exists")
	}

	salt := sm.generateSalt()
	user := User{
		Username: username,
		Password: sm.hashPassword(password, salt),
		Salt:     salt,
		Created:  time.Now(),
		Role:     role,
	}

	sm.users[username] = user
	return sm.saveUsers()
}

func (sm *SecurityManager) Authenticate(username, password string) (string, error) {
	user, exists := sm.users[username]
	if !exists {
		return "", errors.New("user not found")
	}

	hashedPassword := sm.hashPassword(password, user.Salt)
	if hashedPassword != user.Password {
		return "", errors.New("invalid password")
	}

	token := sm.generateToken()
	session := Session{
		Token:     token,
		Username:  username,
		Created:   time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	sm.sessions[token] = session
	return token, nil
}

func (sm *SecurityManager) ValidateSession(token string) (string, error) {
	session, exists := sm.sessions[token]
	if !exists {
		return "", errors.New("session not found")
	}

	if time.Now().After(session.ExpiresAt) {
		delete(sm.sessions, token)
		return "", errors.New("session expired")
	}

	return session.Username, nil
}

func (sm *SecurityManager) Logout(token string) {
	delete(sm.sessions, token)
}

func (sm *SecurityManager) CleanupExpiredSessions() {
	now := time.Now()
	for token, session := range sm.sessions {
		if now.After(session.ExpiresAt) {
			delete(sm.sessions, token)
		}
	}
}
