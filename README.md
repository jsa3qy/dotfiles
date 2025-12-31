# Dotfiles

My personal dotfiles for macOS.

## What's Included

- **Zsh configuration** with Oh My Zsh
  - Theme: robbyrussell
  - Plugins: git, zsh-autosuggestions, zsh-syntax-highlighting, colored-man-pages, web-search, copypath, copyfile, dirhistory, extract, z
  - Custom history settings
  - Directory navigation enhancements
- **Git configuration** with aliases
- **fzf** fuzzy finder integration
- **Homebrew packages** via Brewfile
- **VS Code extensions** via Brewfile

## Installation

On a fresh Mac, clone this repo and run the install script:

```bash
git clone https://github.com/jessealloy/dotfiles.git ~/dotfiles
cd ~/dotfiles
./install.sh
```

The script will:
1. Install Homebrew (if not present)
2. Install all packages from Brewfile
3. Install Oh My Zsh (if not present)
4. Install custom Zsh plugins (autosuggestions, syntax-highlighting)
5. Set up fzf
6. Create symlinks for dotfiles (backing up any existing files)

## Manual Steps After Install

- Set up iTerm2 preferences (if using iTerm2)
- Configure any API keys or credentials
- Sign in to VS Code for settings sync

## Updating

To update the Brewfile with newly installed packages:

```bash
cd ~/dotfiles
brew bundle dump --force
```

Then commit and push the changes.
