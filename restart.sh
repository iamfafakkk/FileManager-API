#!/bin/bash

set -e

echo "🔨 Building filemanager-api..."
go build -o filemanager-api ./cmd/main.go

echo "🔄 Restarting gomanager service..."
sudo systemctl restart gomanager

echo "✅ Done! Service status:"
sudo systemctl status gomanager --no-pager -l
echo ""
echo "📝 To check logs (and specific debug info):"
echo "sudo journalctl -u gomanager -f"
