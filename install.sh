#!/bin/bash

OS="$(uname -s)"
REPO="Hxb8/Termdoro"
LATEST_TAG=$(curl -s "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')

echo "Installing Termdoro ($LATEST_TAG) for $OS..."

if [ -f "/usr/local/bin/termdoro" ]; then
  sudo rm -rf /usr/local/bin/termdoro
fi

if [ "$OS" = "Linux" ]; then
  URL="https://github.com/$REPO/releases/download/$LATEST_TAG/termdoro-linux.tar.gz"
  FILE="termdoro-linux.tar.gz"
elif [ "$OS" = "Darwin" ]; then
  URL="https://github.com/$REPO/releases/download/$LATEST_TAG/termdoro-macos.tar.gz"
  FILE="termdoro-macos.tar.gz"
else
  echo "OS not supported."
  exit 1
fi

curl -L $URL -o $FILE
tar -xzf $FILE

BIN_NAME=$(ls | grep -E 'termdoro' | head -n 1)

if [ -z "$BIN_NAME" ]; then
  echo "Error: Could not find the binary file after extraction."
  exit 1
fi

sudo mv "$BIN_NAME" /usr/local/bin/termdoro
sudo chmod +x /usr/local/bin/termdoro

rm -rf $FILE

echo "Done! Just type 'termdoro' in your terminal to start."
