#!/bin/bash
# Terminal AI - Bash Tab Completion
# Source this file: source terminal-ai-completion.bash

_terminal_ai_commands() {
    cat << 'EOF'
help,version,chat,history,rag,skill,user,provider,web,memory,config
EOF
}

_terminal_ai_rag_commands() {
    cat << 'EOF'
index,add,web,search,status,list,remove,clear
EOF
}

_terminal_ai_chat_commands() {
    cat << 'EOF'
list,new,last,session
EOF
}

_terminal_ai_history_commands() {
    cat << 'EOF'
list,search,recent,clear,dedup
EOF
}

_terminal_ai_memory_commands() {
    cat << 'EOF'
add,recall,list,delete,consolidate,clear
EOF
}

_terminal_ai_skill_commands() {
    cat << 'EOF'
list,create,delete,run
EOF
}

_terminal_ai_user_commands() {
    cat << 'EOF'
list,create,delete,login,logout
EOF
}

_terminal_ai_provider_commands() {
    cat << 'EOF'
list,test,enable,disable,priority,add,default
EOF
}

_terminal_ai_web_commands() {
    cat << 'EOF'
fetch,server
EOF
}

_terminal_ai_config_commands() {
    cat << 'EOF'
list,set,get,unset,reset
EOF
}

_terminal_ai_providers() {
    cat << 'EOF'
openrouter,gemini,groq,byok
EOF
}

_terminal_ai() {
    local cur prev words cword
    _init_completion || return

    local command commands subcommand subcommands

    commands=$(_terminal_ai_commands | tr ',' '\n')

    if [[ ${cword} -eq 1 ]]; then
        COMPREPLY=($(compgen -W "${commands}" -- "${cur}"))
        return 0
    fi

    command="${words[1]}"

    case "${command}" in
        rag)
            subcommands=$(_terminal_ai_rag_commands | tr ',' '\n')
            if [[ ${cword} -eq 2 ]]; then
                COMPREPLY=($(compgen -W "${subcommands}" -- "${cur}"))
            elif [[ ${cword} -eq 3 ]]; then
                if [[ "${words[2]}" == "index" || "${words[2]}" == "add" ]]; then
                    _filedir -d
                elif [[ "${words[2]}" == "web" ]]; then
                    COMPREPLY=($(compgen -W "http:// https://" -- "${cur}"))
                elif [[ "${words[2]}" == "search" ]]; then
                    :
                elif [[ "${words[2]}" == "remove" ]]; then
                    _filedir
                fi
            fi
            ;;

        chat)
            subcommands=$(_terminal_ai_chat_commands | tr ',' '\n')
            if [[ ${cword} -eq 2 ]]; then
                COMPREPLY=($(compgen -W "${subcommands}" -- "${cur}"))
            elif [[ ${cword} -eq 3 ]]; then
                if [[ "${words[2]}" == "session" ]]; then
                    COMPREPLY=($(compgen -W "new list" -- "${cur}"))
                fi
            fi
            ;;

        history)
            subcommands=$(_terminal_ai_history_commands | tr ',' '\n')
            if [[ ${cword} -eq 2 ]]; then
                COMPREPLY=($(compgen -W "${subcommands}" -- "${cur}"))
            elif [[ ${cword} -eq 3 ]]; then
                if [[ "${words[2]}" == "search" ]]; then
                    :
                elif [[ "${words[2]}" == "recent" ]]; then
                    COMPREPLY=($(compgen -W "5 10 20 50" -- "${cur}"))
                fi
            fi
            ;;

        memory)
            subcommands=$(_terminal_ai_memory_commands | tr ',' '\n')
            if [[ ${cword} -eq 2 ]]; then
                COMPREPLY=($(compgen -W "${subcommands}" -- "${cur}"))
            fi
            ;;

        skill)
            subcommands=$(_terminal_ai_skill_commands | tr ',' '\n')
            if [[ ${cword} -eq 2 ]]; then
                COMPREPLY=($(compgen -W "${subcommands}" -- "${cur}"))
            elif [[ ${cword} -eq 3 ]]; then
                if [[ "${words[2]}" == "create" || "${words[2]}" == "delete" || "${words[2]}" == "run" ]]; then
                    COMPREPLY=($(compgen -W "$("${COMP_WORDS[0]}" skill list 2>/dev/null | head -20)" -- "${cur}"))
                fi
            fi
            ;;

        user)
            subcommands=$(_terminal_ai_user_commands | tr ',' '\n')
            if [[ ${cword} -eq 2 ]]; then
                COMPREPLY=($(compgen -W "${subcommands}" -- "${cur}"))
            fi
            ;;

        provider)
            subcommands=$(_terminal_ai_provider_commands | tr ',' '\n')
            if [[ ${cword} -eq 2 ]]; then
                COMPREPLY=($(compgen -W "${subcommands}" -- "${cur}"))
            elif [[ ${cword} -eq 3 ]]; then
                if [[ "${words[2]}" == "add" ]]; then
                    providers=$(_terminal_ai_providers)
                    COMPREPLY=($(compgen -W "${providers}" -- "${cur}"))
                elif [[ "${words[2]}" == "priority" ]]; then
                    COMPREPLY=($(compgen -W "1 2 3" -- "${cur}"))
                elif [[ "${words[2]}" == "enable" || "${words[2]}" == "disable" ]]; then
                    COMPREPLY=($(compgen -W "$("${COMP_WORDS[0]}" provider list 2>/dev/null | head -10)" -- "${cur}"))
                fi
            fi
            ;;

        web)
            subcommands=$(_terminal_ai_web_commands | tr ',' '\n')
            if [[ ${cword} -eq 2 ]]; then
                COMPREPLY=($(compgen -W "${subcommands}" -- "${cur}"))
            elif [[ ${cword} -eq 3 ]]; then
                if [[ "${words[2]}" == "fetch" ]]; then
                    COMPREPLY=($(compgen -W "http:// https://" -- "${cur}"))
                fi
            fi
            ;;

        config)
            subcommands=$(_terminal_ai_config_commands | tr ',' '\n')
            if [[ ${cword} -eq 2 ]]; then
                COMPREPLY=($(compgen -W "${subcommands}" -- "${cur}"))
            fi
            ;;

        help|--help|-h)
            COMPREPLY=($(compgen -W "${commands}" -- "${cur}"))
            ;;
    esac
}

complete -F _terminal_ai terminal-ai

_terminal_ai_raw() {
    echo "${commands}" | tr ',' '\n'
}
