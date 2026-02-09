#!/bin/bash
# Terminal AI - Tab Completion Setup Script
# Run this script to install tab completion for terminal-ai

set -e

echo "🔧 Terminal AI - Tab Completion Setup"
echo "======================================"
echo

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TERMINAL_AI_PATH="$SCRIPT_DIR/terminal-ai"

if [ ! -f "$TERMINAL_AI_PATH" ]; then
    echo "⚠️  terminal-ai not found at: $TERMINAL_AI_PATH"
    echo "   Please build terminal-ai first: cd $SCRIPT_DIR && go build -o terminal-ai ."
    exit 1
fi

echo "📁 Terminal AI path: $TERMINAL_AI_PATH"
echo "📁 Detected shell: $SHELL"
echo

TERMINAL_AI_PATH=$(which terminal-ai 2>/dev/null || echo "/usr/local/bin/terminal-ai")

if [ ! -f "$TERMINAL_AI_PATH" ]; then
    echo "⚠️  terminal-ai not found in PATH"
    echo "   Please make sure terminal-ai is installed first."
    echo "   You can copy this script to the terminal-ai directory."
    exit 1
fi

COMPLETION_DIR_BASH="$HOME/.bash_completion.d"
COMPLETION_DIR_ZSH="$HOME/.zsh/completion"

echo "📁 Detected shell: $SHELL"
echo

# Install bash completion
if [[ "$SHELL" == *"bash"* ]]; then
    echo "📦 Installing Bash completion..."
    
    mkdir -p "$COMPLETION_DIR_BASH"
    cp completion/terminal-ai-completion.bash "$COMPLETION_DIR_BASH/"
    
    # Add to bashrc if not already there
    BASHRC="$HOME/.bashrc"
    COMPLETION_LINE="[ -f $COMPLETION_DIR_BASH/terminal-ai-completion.bash ] && source $COMPLETION_DIR_BASH/terminal-ai-completion.bash"
    
    if ! grep -qF "terminal-ai-completion.bash" "$BASHRC" 2>/dev/null; then
        echo "" >> "$BASHRC"
        echo "# Terminal AI Tab Completion" >> "$BASHRC"
        echo "$COMPLETION_LINE" >> "$BASHRC"
        echo "✅ Added completion to $BASHRC"
    else
        echo "ℹ️  Completion already configured in $BASHRC"
    fi
    
    echo "✅ Bash completion installed!"
fi

# Install zsh completion
if [[ "$SHELL" == *"zsh"* ]]; then
    echo "📦 Installing Zsh completion..."
    
    mkdir -p "$COMPLETION_DIR_ZSH"
    cp completion/terminal-ai-completion.zsh "$COMPLETION_DIR_ZSH/_terminal-ai"
    
    # Add to zshrc if not already there
    ZSHRC="$HOME/.zshrc"
    COMPLETION_LINE="fpath=( $COMPLETION_DIR_ZSH \$fpath )"
    AUTOLOAD_LINE="autoload -Uz compinit && compinit"
    
    if ! grep -qF "terminal-ai" "$ZSHRC" 2>/dev/null; then
        echo "" >> "$ZSHRC"
        echo "# Terminal AI Tab Completion" >> "$ZSHRC"
        echo "$COMPLETION_LINE" >> "$ZSHRC"
        echo "$AUTOLOAD_LINE" >> "$ZSHRC"
        echo "✅ Added completion to $ZSHRC"
    else
        echo "ℹ️  Completion already configured in $ZSHRC"
    fi
    
    echo "✅ Zsh completion installed!"
fi

echo
echo "🎉 Setup complete!"
echo
echo "To start using tab completion:"
if [[ "$SHELL" == *"bash"* ]]; then
    echo "   source ~/.bashrc"
fi
if [[ "$SHELL" == *"zsh"* ]]; then
    echo "   source ~/.zshrc"
fi
echo
echo "Or start a new terminal session."
