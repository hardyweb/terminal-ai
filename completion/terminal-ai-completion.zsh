#compdef terminal-ai
# Terminal AI - Zsh Tab Completion

autoload -U compinit
compinit

_terminal_ai_commands() {
    local commands
    commands=(
        'help:Show help message'
        'version:Show version information'
        'chat:Chat with AI'
        'history:Command history management'
        'rag:RAG (Retrieval Augmented Generation) commands'
        'skill:Custom skills management'
        'user:User management'
        'provider:AI provider configuration'
        'web:Web fetch and server'
        'memory:Long-term memory management'
        'config:Configuration management'
    )
    _describe -t commands 'terminal-ai command' commands
}

_terminal_ai_rag_commands() {
    local rag_commands
    rag_commands=(
        'index:Index a directory (full reindex)'
        'add:Add/update directories (incremental)'
        'web:Scrape and index a webpage'
        'search:Search indexed documents'
        'status:Show index statistics'
        'list:List indexed sources'
        'remove:Remove a source'
        'clear:Clear all RAG data'
    )
    _describe -t rag_commands 'RAG command' rag_commands
}

_terminal_ai_chat_commands() {
    local chat_commands
    chat_commands=(
        'list:List all chat sessions'
        'new:Start a new chat session'
        'last:Continue the last session'
        'session:Manage a specific session'
    )
    _describe -t chat_commands 'chat command' chat_commands
}

_terminal_ai_history_commands() {
    local history_commands
    history_commands=(
        'list:List all commands in history'
        'search:Search history by prefix'
        'recent:Show recent commands'
        'clear:Clear all history'
        'dedup:Remove duplicates'
    )
    _describe -t history_commands 'history command' history_commands
}

_terminal_ai_memory_commands() {
    local memory_commands
    memory_commands=(
        'add:Save to long-term memory'
        'recall:Search memories'
        'list:List all memories'
        'delete:Delete a memory'
        'consolidate:Clean up old memories'
        'clear:Delete ALL memories'
    )
    _describe -t memory_commands 'memory command' memory_commands
}

_terminal_ai_skill_commands() {
    local skill_commands
    skill_commands=(
        'list:List all skills'
        'create:Create a new skill'
        'delete:Delete a skill'
        'run:Run a skill'
    )
    _describe -t skill_commands 'skill command' skill_commands
}

_terminal_ai_user_commands() {
    local user_commands
    user_commands=(
        'list:List all users'
        'create:Create a new user'
        'delete:Delete a user'
        'login:Login as user'
        'logout:Logout current user'
    )
    _describe -t user_commands 'user command' user_commands
}

_terminal_ai_provider_commands() {
    local provider_commands
    provider_commands=(
        'list:List all providers'
        'test:Test provider configuration'
        'enable:Enable a provider'
        'disable:Disable a provider'
        'priority:Set provider priority'
        'add:Add a new provider'
        'default:Set default provider'
    )
    _describe -t provider_commands 'provider command' provider_commands
}

_terminal_ai_web_commands() {
    local web_commands
    web_commands=(
        'fetch:Fetch and process web content'
        'server:Start web server'
    )
    _describe -t web_commands 'web command' web_commands
}

_terminal_ai_config_commands() {
    local config_commands
    config_commands=(
        'list:List all configuration'
        'set:Set a configuration value'
        'get:Get a configuration value'
        'unset:Unset a configuration'
        'reset:Reset to defaults'
    )
    _describe -t config_commands 'config command' config_commands
}

_terminal_ai_providers() {
    local providers
    providers=(
        'openrouter:OpenRouter API'
        'gemini:Google Gemini API'
        'groq:Groq API'
        'byok:Bring Your Own Key'
    )
    _describe -t providers 'AI provider' providers
}

_terminal_ai() {
    local -a args
    
    if (( CURRENT == 2 )); then
        _terminal_ai_commands
        return 0
    fi
    
    local command="${words[2]}"
    
    case "${command}" in
        rag)
            _terminal_ai_rag_commands
            ;;
        chat)
            _terminal_ai_chat_commands
            ;;
        history)
            _terminal_ai_history_commands
            ;;
        memory)
            _terminal_ai_memory_commands
            ;;
        skill)
            _terminal_ai_skill_commands
            ;;
        user)
            _terminal_ai_user_commands
            ;;
        provider)
            _terminal_ai_provider_commands
            ;;
        web)
            _terminal_ai_web_commands
            ;;
        config)
            _terminal_ai_config_commands
            ;;
    esac
}

_terminal_ai
