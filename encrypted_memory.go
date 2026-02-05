package main

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type EncryptedMemoryManager struct {
	base *MemoryManager
}

func NewEncryptedMemoryManager(base *MemoryManager) *EncryptedMemoryManager {
	return &EncryptedMemoryManager{base: base}
}

func (em *EncryptedMemoryManager) AddEncryptedMemory(ctx context.Context, content string, metadata MemoryMetadata) (*Memory, error) {
	if securityMgr == nil {
		return nil, fmt.Errorf("security manager not initialized")
	}

	encryptedContent, err := securityMgr.encrypt(content)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt content: %w", err)
	}

	encryptedMetadata := MemoryMetadata{
		Source:      metadata.Source,
		SessionID:   metadata.SessionID,
		User:        metadata.User,
		Tags:        metadata.Tags,
		IsEncrypted: true,
	}

	memory, err := em.base.AddMemory(ctx, encryptedContent, encryptedMetadata)
	if err != nil {
		return nil, err
	}

	return memory, nil
}

func (em *EncryptedMemoryManager) SearchAndDecrypt(ctx context.Context, query string, topK int) ([]MemorySearchResult, error) {
	results, err := em.base.SearchMemories(ctx, query, topK)
	if err != nil {
		return nil, err
	}

	if securityMgr == nil {
		return results, nil
	}

	for i := range results {
		if results[i].Memory.Metadata.IsEncrypted {
			decryptedContent, err := securityMgr.decrypt(results[i].Memory.Content)
			if err != nil {
				continue
			}
			results[i].Memory.Content = decryptedContent
			results[i].Memory.Metadata.IsEncrypted = false
		}
	}

	return results, nil
}

func (em *EncryptedMemoryManager) GetAndDecrypt(ctx context.Context, id string) (*Memory, error) {
	memory, err := em.base.GetMemory(ctx, id)
	if err != nil {
		return nil, err
	}

	if securityMgr == nil {
		return memory, nil
	}

	if memory.Metadata.IsEncrypted {
		decryptedContent, err := securityMgr.decrypt(memory.Content)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt memory: %w", err)
		}
		memory.Content = decryptedContent
		memory.Metadata.IsEncrypted = false
	}

	return memory, nil
}

func (em *EncryptedMemoryManager) GetAllAndDecrypt(ctx context.Context) ([]Memory, error) {
	memories, err := em.base.GetAllMemories(ctx)
	if err != nil {
		return nil, err
	}

	if securityMgr == nil {
		return memories, nil
	}

	for i := range memories {
		if memories[i].Metadata.IsEncrypted {
			decryptedContent, err := securityMgr.decrypt(memories[i].Content)
			if err != nil {
				continue
			}
			memories[i].Content = decryptedContent
			memories[i].Metadata.IsEncrypted = false
		}
	}

	return memories, nil
}

func (em *EncryptedMemoryManager) ConsolidateEncryptedMemories(ctx context.Context) (int, error) {
	return em.base.ConsolidateMemories(ctx)
}

func (em *EncryptedMemoryManager) DeleteMemory(ctx context.Context, id string) error {
	return em.base.DeleteMemory(ctx, id)
}

func (em *EncryptedMemoryManager) UpdateMemoryImportance(ctx context.Context, id string, importance float32) error {
	return em.base.UpdateMemoryImportance(ctx, id, importance)
}

func (em *EncryptedMemoryManager) SearchByTags(ctx context.Context, tags []string, topK int) ([]MemorySearchResult, error) {
	memories, err := em.base.GetAllMemories(ctx)
	if err != nil {
		return nil, err
	}

	var results []MemorySearchResult
	for _, memory := range memories {
		for _, tag := range tags {
			for _, memoryTag := range memory.Metadata.Tags {
				if strings.Contains(strings.ToLower(memoryTag), strings.ToLower(tag)) {
					results = append(results, MemorySearchResult{
						Memory:     memory,
						Similarity: memory.Importance,
					})
					break
				}
			}
		}
	}

	if len(results) > topK {
		results = results[:topK]
	}

	return results, nil
}

func (em *EncryptedMemoryManager) GetRecentMemories(ctx context.Context, since time.Time, limit int) ([]Memory, error) {
	memories, err := em.base.GetAllMemories(ctx)
	if err != nil {
		return nil, err
	}

	var recent []Memory
	for _, memory := range memories {
		if memory.CreatedAt.After(since) {
			recent = append(recent, memory)
		}
	}

	if len(recent) > limit {
		recent = recent[:limit]
	}

	return recent, nil
}

func (em *EncryptedMemoryManager) GetMemoriesBySource(ctx context.Context, source string) ([]Memory, error) {
	memories, err := em.base.GetAllMemories(ctx)
	if err != nil {
		return nil, err
	}

	var filtered []Memory
	for _, memory := range memories {
		if strings.ToLower(memory.Metadata.Source) == strings.ToLower(source) {
			filtered = append(filtered, memory)
		}
	}

	return filtered, nil
}

var encryptedMemoryMgr *EncryptedMemoryManager

func GetEncryptedMemoryManager() *EncryptedMemoryManager {
	return encryptedMemoryMgr
}

func InitEncryptedMemoryManager(dataDir string) error {
	err := InitMemoryManager(dataDir)
	if err != nil {
		return err
	}

	mgr := GetMemoryManager()
	if mgr != nil {
		encryptedMemoryMgr = NewEncryptedMemoryManager(mgr)
	}

	return nil
}
