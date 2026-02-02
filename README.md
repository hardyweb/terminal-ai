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
```

## Contoh Usage Lengkap

```bash
# Chat biasa
./terminal-ai "Explain RAG"

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
- Pastikan internet connection stabil
- Cuba guna provider lain (gemini/groq biasanya lebih cepat)

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
