#!/bin/bash

# تحديد النظام
OS="$(uname -s)"
ARCH="$(uname -m)"
REPO="hexbyte16/rust-pomo-discord"
LATEST_TAG=$(curl -s "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')

echo "📦 Installing Pomodoro TUI ($LATEST_TAG) for $OS..."

# تحديد رابط التحميل بناءً على النظام
if [ "$OS" = "Linux" ]; then
    URL="https://github.com/$REPO/releases/download/$LATEST_TAG/pomo-linux.tar.gz"
    FILE="pomo-linux.tar.gz"
elif [ "$OS" = "Darwin" ]; then
    URL="https://github.com/$REPO/releases/download/$LATEST_TAG/pomo-macos.tar.gz"
    FILE="pomo-macos.tar.gz"
else
    echo "❌ Your OS is not supported by this script. Download manually from GitHub."
    exit 1
fi

# تحميل وفك الضغط
curl -L $URL -o $FILE
tar -xzf $FILE

# نقل الملف للمسار العالمي
sudo mv rust-pomo-discord /usr/local/bin/pomo
chmod +x /usr/local/bin/pomo

# تنظيف
rm $FILE

echo "✅ Done! Just type 'pomo' in your terminal to start."
