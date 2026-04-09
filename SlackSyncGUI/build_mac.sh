#!/bin/bash

echo "=========================================="
echo "   SlackSyncTool - macOS Build Script"
echo "=========================================="
echo ""
echo "NOTE: This script must be run on a macOS machine."
echo "Wails applications rely on platform-specific libraries (Cocoa/WebKit)"
echo "which cannot be cross-compiled from Windows."
echo ""

# Check for Wails
if command -v wails &> /dev/null; then
    echo "✅ Wails CLI found."
    echo "🚀 Building .app bundle using Wails..."
    wails build
    
    if [ $? -eq 0 ]; then
        echo ""
        echo "✅ Build Successful!"
        echo "👉 Application located in: build/bin/slackdump-gui.app"
    else
        echo "❌ Wails build failed."
        exit 1
    fi
else
    echo "⚠️  Wails CLI not found."
    echo "   It is highly recommended to install Wails: go install github.com/wailsapp/wails/v2/cmd/wails@latest"
    echo ""
    echo "🔄 Attempting fallback manual 'go build'..."
    echo "   (This will produce a binary executable, not a .app bundle)"
    
    go build -tags desktop,production -o SlackSyncTool_Mac
    
    if [ $? -eq 0 ]; then
        echo ""
        echo "✅ Build Successful!"
        echo "👉 Executable: ./SlackSyncTool_Mac"
        chmod +x SlackSyncTool_Mac
    else
        echo "❌ Go build failed."
        exit 1
    fi
fi
