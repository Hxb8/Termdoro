#!/bin/bash

# تحديد النظام
OS="$(uname -s)"
REPO="hexbyte16/rust-pomo-discord"
# جلب آخر نسخة
LATEST_TAG=$(curl -s "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')

echo "📦 Installing Pomodoro TUI ($LATEST_TAG) for $OS..."

if [ -d "/usr/local/bin/pomo" ]; then
    sudo rm -rf /usr/local/bin/pomo
fi

if [ "$OS" = "Linux" ]; then
    URL="https://github.com/$REPO/releases/download/$LATEST_TAG/pomo-linux.tar.gz"
    FILE="pomo-linux.tar.gz"
elif [ "$OS" = "Darwin" ]; then
    URL="https://github.com/$REPO/releases/download/$LATEST_TAG/pomo-macos.tar.gz"
    FILE="pomo-macos.tar.gz"
else
    echo "❌ OS not supported."
    exit 1
fi

curl -L $URL -o $FILE
tar -xzf $FILE

BIN_NAME=$(ls | grep -E 'pomodoro-tui-discord|rust-pomo-discord' | head -n 1)

if [ -z "$BIN_NAME" ]; then
    echo "❌ Error: Could not find the binary file after extraction."
    exit 1
fi

sudo mv "$BIN_NAME" /usr/local/bin/pomo
sudo chmod +x /usr/local/bin/pomo

rm -rf $FILE

echo "✅ Done! Just type 'pomo' in your terminal to start."
