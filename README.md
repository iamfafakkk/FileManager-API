# FileManager API

High-performance FileManager REST API built with Go and Fiber framework.

## Features

- **CRUD Operations** - Create, read, update, delete files and folders
- **Copy/Move** - Copy and move files/folders with batch support
- **Upload with Progress** - Real-time progress via SSE and WebSocket
- **ZIP Compression** - Compress files/folders with progress tracking
- **ZIP Extraction** - Extract archives with progress tracking
- **Usersite Isolation** - Each user sandboxed to `/home/{userSite}`. Created files are automatically owned by the `userSite` system user.
- **SSH Remote Access** - Connect to external servers via SSH/SFTP

## Quick Start

```bash
go mod tidy
go run cmd/main.go
```

Server: `http://localhost:3000`

---

## Headers

### Required Headers (All Requests)

| Header | Description |
|--------|-------------|
| `X-API-Key` | API key (default: `filemanager-secret-key`) |
| `X-User-Site` | Username, path akan jadi `/home/{userSite}` |

### SSH Headers (Optional - untuk remote server)

| Header | Default | Description |
|--------|---------|-------------|
| `X-Ssh-Host` | - | IP/hostname remote server |
| `X-Ssh-Username` | root | SSH username |
| `X-Ssh-Port` | 22 | SSH port |
| `X-Ssh-Key` | - | SSH private key (dengan `\n` untuk newlines) |

**Cara format SSH Key untuk header:**
```bash
cat ~/.ssh/id_rsa | awk '{printf "%s\\n", $0}'
```

---

## API Endpoints

### 1. List Directory

**GET** `/api/v1/fs?path={path}`

Query params:
- `path` - relative path (optional, default: root)

Response:
```json
{
  "success": true,
  "data": [
    {
      "name": "documents",
      "path": "documents",
      "is_dir": true,
      "size": 4096
    },
    {
      "name": "file.txt",
      "path": "file.txt",
      "is_dir": false,
      "size": 1024,
      "extension": "txt",
      "mime_type": "text/plain"
    }
  ]
}
```

---

### 2. Get Info

**GET** `/api/v1/fs/info/{path}`

Response:
```json
{
  "success": true,
  "data": {
    "name": "file.txt",
    "path": "documents/file.txt",
    "size": 1024,
    "is_dir": false,
    "permissions": "rw-r--r--",
    "mod_time": "2026-01-18T12:00:00Z"
  }
}
```

---

### 3. Get Disk Usage

**GET** `/api/v1/fs/disk-usage?path={path}`

Query params:
- `path` - relative path (optional, default: `/home/{userSite}`)

Response:
```json
{
  "success": true,
  "data": {
    "path": "documents",
    "size_bytes": 1048576,
    "size_human": "1.0 MB"
  }
}
```

---

### 4. Download File

**GET** `/api/v1/fs/download/{path}`

Response: File binary dengan headers:
- `Content-Type`: MIME type file
- `Content-Disposition`: attachment

---

### 5. Create File

**POST** `/api/v1/fs/file`

Request Body:
```json
{
  "path": "documents/newfile.txt",
  "content": "Hello World content here"
}
```

Response:
```json
{
  "success": true,
  "message": "File created",
  "data": {
    "name": "newfile.txt",
    "path": "documents/newfile.txt",
    "size": 24
  }
}
```

---

### 6. Update File

**PUT** `/api/v1/fs/file/{path}`

Request Body:
```json
{
  "content": "Updated file content"
}
```

Response:
```json
{
  "success": true,
  "message": "File updated",
  "data": {
    "name": "file.txt",
    "path": "documents/file.txt",
    "size": 20
  }
}
```

---

### 7. Create Folder

**POST** `/api/v1/fs/folder`

Request Body:
```json
{
  "path": "documents/newfolder"
}
```

Response:
```json
{
  "success": true,
  "message": "Folder created",
  "data": {
    "name": "newfolder",
    "path": "documents/newfolder",
    "is_dir": true
  }
}
```

---

### 8. Rename File/Folder

**PUT** `/api/v1/fs/rename/{path}`

Request Body:
```json
{
  "new_name": "renamed-file.txt"
}
```

Response:
```json
{
  "success": true,
  "message": "Renamed successfully",
  "data": {
    "name": "renamed-file.txt",
    "path": "documents/renamed-file.txt"
  }
}
```

---

### 9. Delete File/Folder

**DELETE** `/api/v1/fs/{path}?recursive=true`

Query params:
- `recursive` - `true` untuk hapus folder beserta isinya

Response:
```json
{
  "success": true,
  "message": "Deleted successfully"
}
```

---

### 10. Copy Files/Folders

**POST** `/api/v1/fs/copy`

Request Body:
```json
{
  "sources": ["documents/file1.txt", "documents/file2.txt"],
  "destination": "backup",
  "overwrite": false
}
```

Response:
```json
{
  "success": true,
  "message": "Copied successfully",
  "data": [
    {"name": "file1.txt", "path": "backup/file1.txt"},
    {"name": "file2.txt", "path": "backup/file2.txt"}
  ]
}
```

---

### 11. Move Files/Folders

**POST** `/api/v1/fs/move`

Request Body:
```json
{
  "sources": ["documents/file1.txt", "documents/folder1"],
  "destination": "archive",
  "overwrite": false
}
```
---

### 12. Upload File

**POST** `/api/v1/upload`

Content-Type: `multipart/form-data`

Form fields:
- `file` - File to upload
- `destination` - Target folder (optional)

Response:
```json
{
  "success": true,
  "data": {
    "upload_id": "abc123",
    "progress": {
      "progress": 100,
      "status": "completed"
    }
  }
}
```

---

### 13. Upload Progress (SSE)

**GET** `/api/v1/upload/progress/{upload_id}`

Response: Server-Sent Events stream
```
data: {"progress": 50, "uploaded_bytes": 5000, "total_bytes": 10000, "status": "uploading"}
data: {"progress": 100, "status": "completed"}
```

---

### 14. Compress to ZIP

**POST** `/api/v1/compress`

Request Body:
```json
{
  "paths": ["documents", "photos/image.jpg"],
  "output": "backup.zip",
  "compression_level": 6
}
```

Response:
```json
{
  "success": true,
  "data": {
    "compress_id": "xyz789",
    "output": "backup.zip"
  }
}
```

---

### 15. Extract ZIP

**POST** `/api/v1/extract`

Request Body:
```json
{
  "source": "backup.zip",
  "destination": "extracted"
}
```

Response:
```json
{
  "success": true,
  "data": {
    "extract_id": "def456",
    "destination": "extracted"
  }
}
```

---

### 16. Execute Raw Commands

**POST** `/api/v1/raw`

Execute shell commands within the userSite directory. Commands run with `/home/{userSite}` as working directory.

Request Body:
```json
["pwd", "ls -la", "echo hello"]
```

Response:
```json
{
  "success": true,
  "data": {
    "base_path": "/home/cilik-sd4mg",
    "results": [
      {
        "command": "pwd",
        "output": "/home/cilik-sd4mg",
        "exit_code": 0
      },
      {
        "command": "ls -la",
        "output": "total 48\ndrwxrwx--- 8 cilik-sd4mg...",
        "exit_code": 0
      },
      {
        "command": "echo hello",
        "output": "hello",
        "exit_code": 0
      }
    ]
  }
}
```

**Security Restrictions:**
- Commands execute with `/home/{userSite}` as working directory
- Path traversal (`../`) is blocked
- File operations with absolute paths outside `/home/{userSite}` are blocked
- Dangerous patterns are blocked (e.g., `rm -rf /`, `mkfs`, etc.)

---

## Example: Complete Request dengan SSH

```bash
curl -X GET "http://localhost:3000/api/v1/fs?path=public_html" \
  -H "X-API-Key: filemanager-secret-key" \
  -H "X-User-Site: cilik-sd4mg" \
  -H "X-Ssh-Host: 192.168.1.100" \
  -H "X-Ssh-Port: 22" \
  -H "X-Ssh-Username: root" \
  -H "X-Ssh-Key: -----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaC1rZXktdjEA...\n-----END OPENSSH PRIVATE KEY-----"
```

---

## Error Response Format

```json
{
  "success": false,
  "message": "Error description",
  "error": {
    "code": "ERROR_CODE",
    "details": "Detailed error message"
  },
  "timestamp": "2026-01-18T12:00:00Z"
}
```

Error codes:
- `AUTH_REQUIRED` - Missing API key
- `INVALID_API_KEY` - Wrong API key
- `USERSITE_REQUIRED` - Missing X-User-Site header
- `SSH_ERROR` - SSH connection failed
- `NOT_FOUND` - File/folder not found
- `ALREADY_EXISTS` - File/folder already exists
- `FOLDER_NOT_EMPTY` - Cannot delete non-empty folder
