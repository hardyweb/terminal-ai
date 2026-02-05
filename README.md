# Terminal AI CLI

Simple Go CLI untuk interaksi dengan pelbagai API AI seperti OpenRouter, Gemini, Groq, dan lain-lain.

## Setup (Automatik dengan setup.sh)

Jika menggunakan `setup.sh`:
1. Run setup script: `./setup.sh`
2. Setup akan create semua folder dan file yang diperlukan

## Setup Manual (Tanpa setup.sh)

Jika tidak menggunakan `setup.sh`, anda perlu setup secara manual:

### 1. Install Go
Pastikan Go sudah install di sistem anda.

### 2. Create Configuration Directory

**WAJIB** create folder `~/.config/terminal-ai/` secara manual:

```bash
# Create config directory
mkdir -p ~/.config/terminal-ai

# Create subdirectories untuk user management (WAJIB untuk create user)
mkdir -p ~/.config/terminal-ai/user
```

**Penting:** Folder `~/.config/terminal-ai/user` **MESTI** dibuat secara manual sebelum boleh create user dengan command `terminal-ai user create`.

### 3. Setup Environment Variables

Copy `.env.example` ke `~/.config/terminal-ai/.env`:

```bash
cp .env.example ~/.config/terminal-ai/.env
```

Edit file tersebut dan masukkan API key anda:

```bash
nano ~/.config/terminal-ai/.env
```

### 4. Build Binary

```bash
go mod tidy
go build -o terminal-ai .
```

### 5. Verify Installation

Test dengan command:

```bash
./terminal-ai --help
./terminal-ai provider list
```

## Install Dependencies

```bash
go mod tidy
```

## Build

```bash
go build -o terminal-ai . 
```

## Penggunaan

### Chat dengan AI

```bash
./terminal-ai [provider] <message>
```

Provider default: `openrouter`

Contoh:
```bash
./terminal-ai "Hello, how are you?"
./terminal-ai openrouter "Explain quantum computing"
./terminal-ai gemini "Write a poem"
./terminal-ai groq "Help me with code"
```

### Streaming Mode (Chunk by Chunk Response)

Terminal AI CLI menyokong **streaming responses** - response muncul token-by-token secara real-time:

```bash
# Streaming mode (default - response muncul token by token)
./terminal-ai "Write a story"

# Disable streaming - tunggu response lengkap baru paparkan
./terminal-ai --no-streaming "Write a story"
./terminal-ai -s "Write a story"

# atau guna environment variable
export STREAMING=false
./terminal-ai "Write a story"
```

**Kelebihan Streaming:**
- Response muncul lebih cepat (tak perlu tunggu lengkap)
- Macam chat dengan manusia (token by token)
- Boleh stop bila-bila masa (Ctrl+C)

### Web Fetch Tool

Baca kandungan dari website:

```bash
./terminal-ai web <url>
```

Contoh:
```bash
./terminal-ai web https://example.com
./terminal-ai web https://docs.openclaw.ai
```

### RAG (Retrieval Augmented Generation)

Index dan cari nota-nota lokal anda:

```bash
# Index semua fail dalam direktori
./terminal-ai rag index /path/to/notes

# Cari nota yang berkaitan
./terminal-ai rag search "quantum computing"
```

Nota akan disimpan di `$XDG_DATA_HOME/terminal-ai/rag-index.json` (jika `$XDG_DATA_HOME` diset) atau `$HOME/.local/share/terminal-ai/rag-index.json`

### Skills System

Buat dan manage skills custom:

```bash
# List semua skills
./terminal-ai skill list

# Create skill baru
./terminal-ai skill create <skill-name>
```

Contoh skill creation:
```bash
./terminal-ai skill create summarizer
```

Anda akan diminta untuk input:
- Description: Apa skill ini buat
- Triggers: Kata kunci yang trigger skill ini (comma-separated)
- Template: Prompt template untuk AI

Contoh skill JSON (`~/.terminal-ai/skills/summarizer/skill.json`):
```json
{
  "name": "summarizer",
  "description": "Summarize long text",
  "triggers": ["summarize", "summary", "ringkas"],
  "template": "Please provide a concise summary of the following text:"
}
```

## Provider yang disokong

- `openrouter` - OpenRouter API (default)
- `gemini` - Google Gemini API
- `groq` - Groq API

## Interaksi Berterusan

Selepas mendapat respons, CLI akan tanya sama ada anda ingin teruskan perbualan:
- Tekan `y` untuk teruskan
- Taip message anda (satu baris)
- Tekan Enter
- Ulang langkah di atas untuk terus chat
- Tekan `n` atau Enter kosong untuk keluar

## RAG + Skills Integration

Apabila anda chat:
1. Skills akan auto-trigger jika message mengandungi triggers
2. RAG akan auto-search indexed documents dan tambah ke context

Contoh workflow:
```bash
# 1. Index nota anda
./terminal-ai rag index ~/Documents/notes

# 2. Create skill untuk code review
./terminal-ai skill create code-review
# Description: Review and improve code
# Triggers: review code, check code, code review
# Template: Please review this code and suggest improvements:

# 3. Chat dengan AI
./terminal-ai "review code: function hello() { console.log('hi'); }"
# AI akan gunakan skill code-review dan search nota berkaitan code
```

## Directories

- `~/.config/terminal-ai/` - Main configuration directory **(WAJIB create manual untuk setup tanpa setup.sh)**
- `~/.config/terminal-ai/user/` - User management directory **(WAJIB create manual sebelum boleh create user)**
- `~/.config/terminal-ai/.env` - Environment variables dan API keys
- `~/.config/terminal-ai/providers.json` - Provider configuration
- `~/.config/terminal-ai/skills/` - Custom skills
- `$XDG_DATA_HOME/terminal-ai/rag-index.json` atau `$HOME/.local/share/terminal-ai/rag-index.json` - RAG index cache

**Nota Penting:** Untuk setup manual tanpa `setup.sh`, anda **MESTI** create folder `~/.config/terminal-ai/user/` secara manual sebelum boleh menggunakan command `terminal-ai user create`. Jika folder ini tidak wujud, command create user akan fail.

## Environment Variables

```bash
OPENROUTER_API_KEY=your_key_here
OPENROUTER_ENDPOINT=https://openrouter.ai/api/v1/chat/completions
OPENROUTER_MODEL=meta-llama/llama-3.2-3b-instruct:free

GEMINI_API_KEY=your_key_here
GEMINI_ENDPOINT=https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent
GEMINI_MODEL=gemini-2.0-flash

GROQ_API_KEY=your_key_here
GROQ_ENDPOINT=https://api.groq.com/openai/v1/chat/completions
GROQ_MODEL=llama-3.3-70b-versatile

# Streaming mode (true/false)
# true = streaming (chunk by chunk), false = single response
STREAMING=true
```

## Timeout Handling

### CLI Timeout

CLI streaming ada timeout yang panjang untuk response panjang:

- **Timeout:** 5 minit (300 saat)
- **Activity monitoring:** Warning jika tiada activity selama 2 minit
- **Continue option:** Boleh tekan Enter untuk sambung tunggu

```bash
# Jika timeout warning keluar:
⚠️  No activity for 2 minutes... (stream may be slow)
   Press Enter to continue waiting, or Ctrl+C to cancel
```

### Web UI Timeout Recovery

Web UI ada fitur timeout recovery untuk response panjang:

1. **Progress saving:** Simpan progress ke localStorage setiap chunk
2. **Timeout detection:** Auto-detect bila connection timeout
3. **Continue button:** Papar button "Continue Response" jika timeout
4. **Resume capability:** Sambung dari posisi terakhir

```bash
# Jika stream timeout dalam Web UI:
- Partial response dipaparkan
- Button "Continue Response" muncul
- Klik untuk sambung dari posisi terakhir
```

### Tips untuk Response Panjang

```bash
# 1. Gunakan --no-streaming untuk response sekali gus
./terminal-ai --no-streaming "Write a 5000 word article"

# 2. Gunakan provider yang lebih cepat (Groq)
./terminal-ai groq "Write a long article"

# 3. Bahagi soalan panjang kepada beberapa bahagian
./terminal-ai "Part 1: Explain quantum computing"
./terminal-ai "Part 2: What are the applications?"

# 4. Jika timeout dalam Web UI, klik "Continue Response"
```

## Contoh Usage Lengkap

```bash
# Chat biasa (streaming - token by token)
./terminal-ai "Explain RAG"

# Chat tanpa streaming (tunggu response lengkap)
./terminal-ai --no-streaming "Explain RAG"
./terminal-ai -s "Write a long article"

# Fetch web content
./terminal-ai web https://en.wikipedia.org/wiki/Quantum_computing

# Index nota projek
./terminal-ai rag index ~/projects/docs

# Cari nota berkaitan
./terminal-ai rag search "API authentication"

# Create skill untuk documentation
./terminal-ai skill create doc-writer
# Description: Write technical documentation
# Triggers: document, documentation, docs
# Template: Write clear technical documentation for:

# Chat dengan skill + RAG
./terminal-ai "document the authentication system"
```

## Troubleshooting

### Jika timeout error:

**CLI Timeout:**
```bash
# Cuba gunakan --no-streaming untuk response sekali gus
./terminal-ai --no-streaming "Long request"

# Cuba provider lain yang lebih cepat
./terminal-ai groq "Your request"
```

**Web UI Timeout:**
- Jika timeout, response partial akan dipaparkan
- Klik button "Continue Response" untuk sambung
- Progress disimpan secara automatik

**Tips:**
- Gunakan `--no-streaming` untuk response yang sangat panjang
- Gunakan Groq (paling cepat) untuk request panjang
- Bahagi request panjang kepada beberapa bahagian kecil

### Jika API key error:
- Check `~/.config/terminal-ai/.env` file
- Pastikan API key betul dan ada credit

### Jika tidak boleh create user (setup manual tanpa setup.sh):
**Error:** `Failed to create user: open /home/user/.config/terminal-ai/user/users.json: no such file or directory`

**Solution:** Create folder user secara manual:
```bash
mkdir -p ~/.config/terminal-ai/user
```

Selepas itu, barulah boleh create user:
```bash
./terminal-ai user create username admin
```

### Jika provider configuration tidak dijumpai:
- Pastikan folder `~/.config/terminal-ai/` telah dibuat
- Run `./terminal-ai provider list` untuk verify
- Jika masih fail, create folder secara manual: `mkdir -p ~/.config/terminal-ai`

### Setup Manual Checklist:
Jika menggunakan setup manual (tanpa setup.sh), pastikan:
- [ ] Folder `~/.config/terminal-ai/` telah dibuat
- [ ] Folder `~/.config/terminal-ai/user/` telah dibuat (untuk user management)
- [ ] File `~/.config/terminal-ai/.env` telah di-copy dari `.env.example`
- [ ] API keys telah dimasukkan dalam `.env`
- [ ] Binary telah di-build: `go build -o terminal-ai .`

## Memory System (ChatGPT-like Long-term Memory)

Terminal AI CLI menyokong sistem memory yang mengecam, mencari, dan simpan maklumat penting dari perbualan anda secara automatik.

### Command Memory

```bash
# Tambah memory manually
./terminal-ai memory add "Nama saya Ahmad, saya seorang software engineer"

# Cari memory berdasarkan semantic meaning
./terminal-ai memory recall "siapa nama saya"

# List semua memories
./terminal-ai memory list

# Padam memory tertentu
./terminal-ai memory delete 1

# Clear semua memories
./terminal-ai memory clear

# Consolidate - padam memories lama/rendah importance
./terminal-ai memory consolidate
```

### Auto-Extraction (Otomatik)

Setiap kali anda chat, AI akan auto-extract maklumat penting dari perbualan dan simpan sebagai memory:

```bash
echo "Saya kerja sebagai data scientist dan suka Python" | ./terminal-ai openrouter
# AI akan auto-simpan:
# - Occupation: Data scientist
# - Preference: Python programming language
```

### Memory Encryption

Semua memories disulitkan dengan AES-256 encryption sebelum disimpan dalam vector database.

### Cara Penggunaan

```bash
# 1. Tambah memory
./terminal-ai memory add "Saya alergic terhadap seafood"

# 2. Chat seperti biasa - memories akan auto-extract
echo "My name is Sarah" | ./terminal-ai openrouter

# 3. Cuba recall
./terminal-ai memory recall "siapa nama saya"
# Result: Sarah
./terminal-ai memory recall "apa makanan yang saya tak boleh makan"
# Result: Seafood
```

## Ollama Local Embeddings (Percuma & Privacy)

Anda boleh guna Ollama untuk embeddings secara percuma (tiada API costs, data tak hantar ke luar).

### Install Ollama

```bash
# Install Ollama dari https://ollama.com
curl -fsSL https://ollama.com/install.sh | sh

# Pull model untuk embeddings (nomic-embed-text)
ollama pull nomic-embed-text
```

### Setup di `.env`

Edit `~/.config/terminal-ai/.env`:

```bash
# Enable Ollama embeddings
USE_OLLAMA_EMBEDDINGS=true
OLLAMA_EMBEDDINGS_URL=http://localhost:11434/api/embeddings
OLLAMA_EMBEDDINGS_MODEL=nomic-embed-text
```

### Model Ollama untuk Embeddings

| Model | Saiz | Keterangan |
|-------|------|------------|
| `nomic-embed-text` | ~274MB | Bagus untuk general use |
| `mxbai-embed-large` | ~1.3GB | Kualiti terbaik |
| `all-minilm` | ~90MB | Paling ringan |

### Cara Guna

```bash
# Pastikan Ollama sedang berjalan
ollama serve

# Tambah memory (guna Ollama embeddings)
./terminal-ai memory add "Test memory"

# Chat dengan auto-extraction
echo "My name is John" | ./terminal-ai openrouter

# Recall memories
./terminal-ai memory recall "nama"
```

### Troubleshooting Ollama

**Ollama tak responding:**
```bash
# Check sama ada Ollama sedang berjalan
curl http://localhost:11434/api/tags

# Jika tak running, start Ollama
ollama serve
```

**Vector dimension error:**
```bash
# Ini berlaku jika switch antara OpenRouter dan Ollama
# Clear database lama
rm -rf ~/.local/share/terminal-ai/memory/

# Cuba lagi
./terminal-ai memory add "Test"
```

**Model tak dijumpai:**
```bash
# Pull model yang betul
ollama pull nomic-embed-text

# List model yang ada
ollama list
```
