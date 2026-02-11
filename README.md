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

### Quick Start

```bash
# Start semua services (web + terminal + telegram)
./terminal-ai start

# Akses dari browser
# Web UI: http://localhost:8181/
# Terminal: http://localhost:8181/terminal
```

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

Index dan cari nota-nota lokal anda dengan hybrid search (vector + keyword):

```bash
# Enhanced RAG Commands
./terminal-ai rag add ~/Documents/notes          # Tambah direktori
./terminal-ai rag add ~/Projects/docs             # Tambah multiple direktori
./terminal-ai rag list                            # Senarai sources
./terminal-ai rag status                          # Status index
./terminal-ai rag search "query"                  # Hybrid search
./terminal-ai rag search "query" --type web      # Filter web sources
./terminal-ai rag remove ~/Documents/notes       # Buang source
./terminal-ai rag clear                          # Clear semua index
./terminal-ai rag web https://example.com         # Index web page
```

**Hybrid Search Features:**
- 60% Vector similarity + 40% Keyword (BM25)
- Semantic search + exact keyword matching
- Top 20 results fused dan ranked

**Progress Indicator:**
Semasa indexing, progress bar akan tunjuk status:
```
➕ Incremental update for: ~/Documents/notes
  ⠋ 15/65 chunks (23%)
```

**Optimized for Low RAM Systems:**
- Chunk size: 500 karakter (default) - boleh tukar dalam config
- Skip files >5MB automatically
- Single worker untuk Ollama embeddings (tak overwhelm CPU)
- Progress update setiap chunk

**Incremental Updates:**
- SHA-256 hash tracking untuk setiap file
- Hanya file yang berubah akan di-reindex
- Efisien untuk folder besar

**Web Scraping:**
- Static HTML scraping tanpa JavaScript
- Auto-index paragraph dan headings
- Boleh update web sources yang sama

Nota akan disimpan di `$XDG_DATA_HOME/terminal-ai/` atau `$HOME/.local/share/terminal-ai/`

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

## Chat Session Management

Manage chat sessions dengan persistent history:

```bash
# List semua chat sessions
./terminal-ai chat --list

# Start chat session baru (REPL mode)
./terminal-ai chat --new

# Start chat session baru dengan mesej awal
./terminal-ai chat --new "Explain quantum computing"

# Sambung session terakhir
./terminal-ai chat --last

# Sambung session terakhir dengan mesej
./terminal-ai chat --last "Continue from where we left off"

# Sambung session tertentu (guna ID dari --list)
./terminal-ai chat --session <session-id>
./terminal-ai chat --session abc123 "Hello again"
```

### Chat Session Features

- **Persistent History:** Semua chat disimpan dan boleh disambung bila-bila masa
- **Auto-Memory:** AI auto-extract dan simpan maklumat penting dari setiap perbualan (max 10 memories per chat)
- **REPL Loop:** Setelah mesej pertama, anda boleh teruskan perbualan dalam interactive mode
- **Multiple Sessions:** Boleh ada banyak chat sessions yang berbeza untuk topik berbeza

### Chat Session Workflow

```bash
# 1. Start chat baru tentang projek
./terminal-ai chat --new "Help me design a database schema for e-commerce"

# 2. REPL loop akan bermula - boleh teruskan perbualan
# 📝 Response (streaming):
# [AI response appears here]
# 
# 💾 Extracting important information...
# ✅ Saved 3 memories
#
# Do you want to continue? (y/n): y
# You: What about payment integration?

# 3. Keluar dengan 'n' atau Enter
# Chat session saved: sess_abc123

# 4. List sessions untuk cari session ID
./terminal-ai chat --list

# 5. Sambung session lain hari
./terminal-ai chat --session sess_abc123 "Let's continue with the payment design"
```

## Interactive Chat Mode

Masuk ke mod chat berterusan secara langsung (tanpa save session):

```bash
./terminal-ai interactive
./terminal-ai interactive --provider groq
```

### Commands dalam Interactive Mode

| Command | Description |
|---------|-------------|
| `/help`, `?` | Show help |
| `/quit`, `/exit` | Exit chat |
| `/clear` | Clear screen |
| `/history` | Show chat history |
| `/stats` | Session statistics |
| `/provider <name>` | Switch AI provider |
| `/model <name>` | Set model |
| `/rag on\|off` | Enable/disable RAG context |
| `/rag` | Show RAG status |
| `/index <dir>` | Index a directory |
| `/search <query>` | Search indexed documents |
| `/tokens` | Token tracking (placeholder) |

### RAG Context dalam Interactive Mode

Mod ini menyokong RAG secara langsung - tanya soalan tentang dokumen yang telah di-index:

```bash
./terminal-ai interactive

# Dalam chat:
➤ You: Apa yang sayacatat tentang ModSecurity?
# AI akan cari dalam indexed documents dan jawab

➤ You: /index ~/Documents/notes
# Index direktori baru

➤ You: /search nginx
# Cari dokumen berkaitan nginx
```

**Status RAG dipaparkan di welcome message:**
```
RAG: ✅ Active | Sources: 237 | Chunks: 1602
```

### Streaming dalam Interactive Mode

Response muncul token-by-token secara real-time dengan animasi:

```
🚀🚀🚀 Streaming from openrouter 🚀🚀🚀
[response muncul satu per satu]
✨ Response Complete ✨
```

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
- `~/.config/terminal-ai/completion/` - Shell completion scripts
- `$XDG_DATA_HOME/terminal-ai/` atau `$HOME/.local/share/terminal-ai/` - RAG index, vector database, chat history

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

# Telegram Bot (Optional)
TELEGRAM_BOT_TOKEN=your_telegram_bot_token
TELEGRAM_WEBHOOK_URL=https://your-domain.com
TELEGRAM_ALLOWED_USERS=123456789,987654321
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

# Enhanced RAG Commands
./terminal-ai rag add ~/projects/docs              # Tambah direktori
./terminal-ai rag search "API authentication"      # Hybrid search
./terminal-ai rag web https://example.com          # Index web page

# Chat Session Management (Persistent Sessions)
./terminal-ai chat --list                              # List semua sessions
./terminal-ai chat --new "Help me with Python"        # Start session baru
./terminal-ai chat --last                             # Sambung session terakhir
./terminal-ai chat --session abc123 "Continue"       # Sambung session tertentu

# Interactive Chat Mode dengan RAG
./terminal-ai interactive
# ➤ You: Apa yang saya catat tentang Docker?
# ➤ You: /index ~/new-notes
# ➤ You: /search kubernetes
```

## Troubleshooting

### Interactive Mode Issues

**Tiada response:**
```bash
# Check provider configuration
./terminal-ai provider list

# Cuba provider lain
./terminal-ai interactive --provider groq
```

**RAG tak berfungsi dalam interactive mode:**
```bash
# Check RAG status
./terminal-ai rag status

# Index documents dulu
./terminal-ai rag add ~/documents

# Cuba dalam interactive mode
./terminal-ai interactive
# ➤ You: /rag on
```

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

### RAG Performance Issues (Low RAM Systems)

**Indexing terlalu lambat atau hang:**

```bash
# Gunakan Ollama untuk embeddings secara local
export USE_OLLAMA_EMBEDDINGS=true
export OLLAMA_EMBEDDINGS_URL=http://localhost:11434/api/embeddings
export OLLAMA_EMBEDDINGS_MODEL=nomic-embed-text

# Start Ollama dengan model ringan
ollama serve
ollama pull nomic-embed-text

# Kemudian index
./terminal-ai rag add ~/documents
```

**Tips untuk low RAM:**
- Chunk size default 500 karakter (lebih kecil = lebih laju)
- Files >5MB akan di-skip automatik
- Gunakan single worker (tak overwhelm CPU)
- Progress akan tunjuk setiap chunk: `⠋ 15/65 chunks (23%)`

### Chat --new Stuck After Memory Save

**Masalah:** Lepas "Saved X memories", program nampak stuck

**Sebenarnya:** Program tunggu input "Continue? (y/n):" tapi prompt tak display

**Solution:**
```bash
# Taip 'n' dan Enter untuk exit
# Atau taip 'y' untuk sambung chat

# Jika nak terus exit selepas satu mesej:
./terminal-ai "Your message"  # Guna tanpa --new
```

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

Setiap kali anda chat, AI akan auto-extract maklumat penting dari perbualan dan simpan sebagai memory (max 10 per chat untuk performance):

```bash
./terminal-ai chat --new "Saya kerja sebagai data scientist dan suka Python"
# AI akan auto-simpan:
# - Occupation: Data scientist
# - Preference: Python programming language
```

### Memory Features

- **Smart Extraction:** AI extract hanya maklumat penting (personal info, preferences, project details)
- **Deduplication:** Elak duplicate memories
- **Limit:** Max 10 memories per chat extraction (elak overload)
- **Fast Processing:** Optimized untuk low RAM systems

### Memory Encryption

Semua memories disulitkan dengan AES-256 encryption sebelum disimpan dalam vector database.

### Cara Penggunaan

```bash
# 1. Tambah memory
./terminal-ai memory add "Saya alergic terhadap seafood"

# 2. Chat seperti biasa - memories akan auto-extract
./terminal-ai chat --new "My name is Sarah"

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

## Telegram Bot Integration

Chat dengan Terminal AI terus dari Telegram.

### Setup

1. **Buat bot dari @BotFather:**
   - Buka Telegram, cari @BotFather
   - Taip `/newbot`
   - Masukkan nama dan username bot
   - Copy token yang diberikan

2. **Set environment variable:**
```bash
export TELEGRAM_BOT_TOKEN="your_bot_token_here"
```

3. **Start bot:**
```bash
./terminal-ai telegram start
```

### Bot Commands

| Command | Description |
|---------|-------------|
| `/help` | Show help |
| `/chat <message>` | Chat dengan AI |
| `/interactive` | Interactive mode |
| `/provider list/set` | Manage providers |
| `/rag on/off/search` | RAG operations |
| `/memory add/recall` | Memory operations |
| `/web <url>` | Fetch web page |
| `/status` | Show status |

### User Management

| Command | Description |
|---------|-------------|
| `/userinfo <id>` | Maklumat user |
| `/allow <id>` | Benarkan user |
| `/block <id>` | Sekat user |
| `/userlist` | Senarai users |

### User Access Control

**Option A: Whitelist (hanya user tertentu boleh guna)**
```bash
export TELEGRAM_ALLOWED_USERS=123456789,987654321
./terminal-ai telegram start
```

**Option B: Open (s siapa boleh, admin filter)**
```bash
# Tak set TELEGRAM_ALLOWED_USERS
./terminal-ai telegram start

# Admin boleh allow/block user
/allow 123456789
/block 123456789
```

## Remote Terminal (xterm.js)

Akses terminal server dari browser (PC, phone, tablet).

### Start Web Server

```bash
# Start web + terminal
./terminal-ai start

# Atau web server sahaja
./terminal-ai web-server
```

### Access Terminal

1. Buka browser: `http://localhost:8181/terminal`
2. Login dengan username/password
3. Terminal akan terbuka dalam browser

### Features

- Full xterm.js terminal emulation
- WebSocket real-time communication
- Multiple sessions support
- Command security filtering
- Auto-reconnect
- Mobile-friendly UI

### Toolbar Actions

| Button | Action |
|--------|--------|
| 📺 New Session | Buka session baru |
| 🧹 Clear | Clear terminal |
| ⛶ Fullscreen | Fullscreen mode |
| ❌ Disconnect | Putuskan connection |
| ℹ️ Info | Session info |

### Commands Diblok (Security)

Command berbahaya akan ditolak:
- `rm -rf /`, `mkfs`
- Fork bombs
- `sudo`, `su`
- `chmod/chown 777`
- `wget|curl | sh`
- Netcat reverse shells

## Start All Services

Start web server, terminal, dan telegram bot sekali gus:

```bash
# Web + Terminal sahaja
./terminal-ai start

# Web + Terminal + Telegram
export TELEGRAM_BOT_TOKEN="your_token"
./terminal-ai start
```

**Output:**
```
🚀 Starting all services...

📦 Starting Web Server...
   🌐 Web UI:      http://localhost:8181/
   📟 Terminal:    http://localhost:8181/terminal
   🔌 API:         http://localhost:8181/api

📱 Starting Telegram Bot...
   ✅ Telegram bot running (polling mode)

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
✅ All services started successfully!
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

## Remote Access dengan Cloudflare Tunnel

Akses dari luar network (phone, office, etc) tanpa port forwarding.

### Install cloudflared (Linux):
```bash
curl -L https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-amd64 -o cloudflared
chmod +x cloudflared
```

### Start Tunnel:
```bash
./cloudflared tunnel --url http://localhost:8181/terminal
```

**Output:**
```
2024-01-01T00:00:00Z INF Starting tunnel...
INF Tunnel established at https://random-name.trycloudflare.com
```

### Access:
- **Web UI:** `https://your-tunnel.trycloudflare.com/`
- **Terminal:** `https://your-tunnel.trycloudflare.com/terminal`
- **Telegram:** Buka bot di Telegram

**Nota:** Cloudflare tunnel perlu running untuk akses remote. Auto-restart dengan systemd jika perlu.

## Changelog

### Latest

**Fixed:**
- Fixed RAG worker pool bug - chunks now distributed correctly instead of processing 4x redundantly
- Fixed REPL loop stuck issue - stdout now flushes properly after memory extraction
- Fixed memory extraction - now handles AI responses better and limits to 10 memories per chat

**Added:**
- RAG progress bar during indexing (`⠋ 15/65 chunks (23%)`)
- Memory extraction optimization (max 10 memories, deduplication)
- Low RAM optimizations (500 char chunks, skip files >5MB, single worker)
- Better error handling for Ollama embeddings

**Changed:**
- Reduced default chunk size from 1000 to 500 characters for better performance
- Increased Ollama timeout to 30 minutes for slow systems
- Disabled debug output for cleaner UX

### Previous

- Chat session management (`chat --list`, `--new`, `--last`, `--session`)
- Persistent chat sessions with auto-memory extraction
- REPL mode for continuing conversations

## License

MIT License - Feel free to use and modify.
