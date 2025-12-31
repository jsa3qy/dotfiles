#!/bin/bash

# Dotfiles installation script
# Run this on a fresh Mac to set up your environment

set -e

DOTFILES_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "Installing dotfiles from $DOTFILES_DIR"

# Install Homebrew if not present
if ! command -v brew &> /dev/null; then
    echo "Installing Homebrew..."
    /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"

    # Add Homebrew to PATH for Apple Silicon Macs
    if [[ $(uname -m) == "arm64" ]]; then
        echo 'eval "$(/opt/homebrew/bin/brew shellenv)"' >> ~/.zprofile
        eval "$(/opt/homebrew/bin/brew shellenv)"
    fi
fi

# Install packages from Brewfile
echo "Installing Homebrew packages..."
brew bundle --file="$DOTFILES_DIR/Brewfile"

# Install Oh My Zsh if not present
if [ ! -d "$HOME/.oh-my-zsh" ]; then
    echo "Installing Oh My Zsh..."
    sh -c "$(curl -fsSL https://raw.githubusercontent.com/ohmyzsh/ohmyzsh/master/tools/install.sh)" "" --unattended
fi

# Install custom Oh My Zsh plugins
ZSH_CUSTOM="${ZSH_CUSTOM:-$HOME/.oh-my-zsh/custom}"

if [ ! -d "$ZSH_CUSTOM/plugins/zsh-autosuggestions" ]; then
    echo "Installing zsh-autosuggestions..."
    git clone https://github.com/zsh-users/zsh-autosuggestions "$ZSH_CUSTOM/plugins/zsh-autosuggestions"
fi

if [ ! -d "$ZSH_CUSTOM/plugins/zsh-syntax-highlighting" ]; then
    echo "Installing zsh-syntax-highlighting..."
    git clone https://github.com/zsh-users/zsh-syntax-highlighting "$ZSH_CUSTOM/plugins/zsh-syntax-highlighting"
fi

# Set up fzf
if command -v fzf &> /dev/null; then
    echo "Setting up fzf..."
    "$(brew --prefix)/opt/fzf/install" --key-bindings --completion --no-update-rc --no-bash --no-fish
fi

# Create symlinks for dotfiles
echo "Creating symlinks..."

# Backup existing files if they exist and aren't symlinks
backup_and_link() {
    local source="$1"
    local target="$2"

    if [ -e "$target" ] && [ ! -L "$target" ]; then
        echo "Backing up existing $target to $target.backup"
        mv "$target" "$target.backup"
    fi

    if [ -L "$target" ]; then
        rm "$target"
    fi

    ln -s "$source" "$target"
    echo "Linked $source -> $target"
}

backup_and_link "$DOTFILES_DIR/.zshrc" "$HOME/.zshrc"
backup_and_link "$DOTFILES_DIR/.gitconfig" "$HOME/.gitconfig"

echo ""
echo "Installation complete!"
echo "Please restart your terminal or run: source ~/.zshrc"
