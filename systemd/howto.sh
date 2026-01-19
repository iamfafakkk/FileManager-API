# 1. Build binary (jika belum)
cd /root/GOLANG/FILEMANAGER-API
go build -o filemanager-api ./cmd/main.go

# 2. Copy service file ke systemd
sudo cp /root/GOLANG/FILEMANAGER-API/systemd/gomanager.service /etc/systemd/system/

# 3. Reload systemd
sudo systemctl daemon-reload

# 4. Enable service (auto start saat boot)
sudo systemctl enable gomanager

# 5. Start service
sudo systemctl start gomanager

# 6. Check status
sudo systemctl status gomanager

# 7. Lihat logs
sudo journalctl -u gomanager -f