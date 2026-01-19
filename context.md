# FileManager API - Golang

## Prompt untuk Pengembangan

Buatkan **FileManager REST API** menggunakan **Golang** dengan fitur lengkap dan performa tinggi.

---

## 🎯 Requirements Utama

### 1. CRUD Operations (File & Folder)
- **Create**: Buat file baru dan folder baru
- **Read**: List files/folders dengan pagination, get file content, get file/folder info (size, modified date, permissions)
- **Update**: Rename file/folder, move file/folder ke lokasi lain
- **Delete**: Hapus file/folder (dengan opsi recursive untuk folder)

### 2. Copy & Paste
- Copy file/folder ke lokasi lain
- Support copy multiple files sekaligus
- Preservasi metadata (timestamps, permissions)
- Handle duplicate filename (auto rename dengan suffix)

### 3. Upload dengan Progress Callback
- Endpoint upload file dengan **real-time progress percentage**
- Gunakan **Server-Sent Events (SSE)** atau **WebSocket** untuk streaming progress
- Support **chunked upload** untuk file besar
- Response format:
```json
{
  "filename": "document.pdf",
  "progress": 75,
  "uploaded_bytes": 7500000,
  "total_bytes": 10000000,
  "status": "uploading"
}
```

### 4. Compress (ZIP)
- Compress single file ke ZIP
- Compress multiple files ke satu ZIP
- Compress folder beserta isinya (recursive)
- Progress callback untuk compression
- Support compression level setting

### 5. Extract (ZIP)
- Extract ZIP ke folder tujuan
- Progress callback untuk extraction
- Handle nested folders dalam ZIP
- Option: extract to new folder atau current directory

---

## ⚡ Performance Requirements

### Wajib Implementasi:
1. **Goroutines & Concurrency**
   - Parallel processing untuk operasi batch
   - Worker pool untuk handle multiple requests
   - Non-blocking I/O operations

2. **Streaming untuk File Besar**
   - Gunakan `io.Reader`/`io.Writer` streaming
   - Jangan load seluruh file ke memory
   - Buffer size yang optimal (32KB - 64KB)

3. **Efficient File Operations**
   - Gunakan `io.Copy` dengan buffer
   - Syscall optimizations dimana memungkinkan
   - Memory-mapped files untuk operasi tertentu

4. **Caching**
   - Cache directory listings
   - Cache file metadata
   - Invalidate cache saat ada perubahan

---

## 🏗️ Tech Stack yang Direkomendasikan

```
Framework    : Fiber / Gin / Echo (pilih salah satu - Fiber untuk performa terbaik)
Router       : Built-in dari framework
Concurrency  : Native Go goroutines + sync package
Compression  : archive/zip (standard library)
WebSocket    : gorilla/websocket atau fiber/websocket
Config       : viper
Logging      : zerolog / zap (high-performance logger)
Validation   : go-playground/validator
```

---

## 📁 Struktur Project yang Diharapkan

```
filemanager-api/
├── cmd/
│   └── main.go
├── internal/
│   ├── config/
│   │   └── config.go
│   ├── handlers/
│   │   ├── file_handler.go
│   │   ├── folder_handler.go
│   │   ├── upload_handler.go
│   │   ├── compress_handler.go
│   │   └── extract_handler.go
│   ├── services/
│   │   ├── file_service.go
│   │   ├── folder_service.go
│   │   ├── upload_service.go
│   │   ├── compress_service.go
│   │   └── extract_service.go
│   ├── models/
│   │   ├── file.go
│   │   ├── response.go
│   │   └── progress.go
│   ├── middleware/
│   │   ├── auth.go
│   │   ├── cors.go
│   │   └── ratelimit.go
│   └── utils/
│       ├── fileutils.go
│       └── pathutils.go
├── pkg/
│   └── progresswriter/
│       └── writer.go
├── go.mod
├── go.sum
├── .env.example
└── README.md
```

---

## 🔌 API Endpoints

### Files
```
GET     /api/v1/files              - List files (dengan pagination & filter)
GET     /api/v1/files/:path        - Get file info
GET     /api/v1/files/:path/content - Download/read file content
POST    /api/v1/files              - Create new file
PUT     /api/v1/files/:path        - Update/rename file
DELETE  /api/v1/files/:path        - Delete file
POST    /api/v1/files/copy         - Copy file(s)
POST    /api/v1/files/move         - Move file(s)
```

### Folders
```
GET     /api/v1/folders            - List folders
GET     /api/v1/folders/:path      - Get folder info & contents
POST    /api/v1/folders            - Create new folder
PUT     /api/v1/folders/:path      - Rename folder
DELETE  /api/v1/folders/:path      - Delete folder (recursive option)
POST    /api/v1/folders/copy       - Copy folder
POST    /api/v1/folders/move       - Move folder
```

### Upload
```
POST    /api/v1/upload             - Upload file (multipart)
POST    /api/v1/upload/chunked     - Chunked upload
GET     /api/v1/upload/progress/:id - SSE endpoint untuk progress (EventStream)
WS      /api/v1/upload/ws/:id      - WebSocket untuk real-time progress
```

### Compression
```
POST    /api/v1/compress           - Compress files/folder ke ZIP
GET     /api/v1/compress/progress/:id - Progress compression
POST    /api/v1/extract            - Extract ZIP file
GET     /api/v1/extract/progress/:id  - Progress extraction
```

---

## 📝 Request/Response Examples

### Upload dengan Progress
```bash
# Start upload
POST /api/v1/upload
Content-Type: multipart/form-data

# Listen progress via SSE
GET /api/v1/upload/progress/abc123
Accept: text/event-stream

# SSE Response stream:
data: {"progress": 10, "uploaded": 1048576, "total": 10485760}
data: {"progress": 50, "uploaded": 5242880, "total": 10485760}
data: {"progress": 100, "uploaded": 10485760, "total": 10485760, "status": "completed"}
```

### Compress Request
```json
POST /api/v1/compress
{
  "paths": ["/documents/file1.pdf", "/documents/folder1"],
  "output": "/archives/backup.zip",
  "compression_level": 6
}
```

### Copy Files Request
```json
POST /api/v1/files/copy
{
  "sources": ["/documents/file1.pdf", "/documents/file2.pdf"],
  "destination": "/backup/",
  "overwrite": false
}
```

---

## 🔒 Security Considerations
- Path traversal prevention (sanitize semua path input)
- File size limits
- Allowed file extensions (optional whitelist)
- Rate limiting untuk upload
- Authentication middleware (Fixed API Key)

---

## 📊 Response Format Standard

```json
{
  "success": true,
  "message": "Operation completed successfully",
  "data": { },
  "error": null,
  "timestamp": "2024-01-18T12:00:00Z"
}
```

### Error Response
```json
{
  "success": false,
  "message": "File not found",
  "data": null,
  "error": {
    "code": "FILE_NOT_FOUND",
    "details": "The requested file does not exist"
  },
  "timestamp": "2024-01-18T12:00:00Z"
}
```

---

## 🚀 Prioritas Implementasi

1. **Phase 1**: Setup project, config, basic CRUD
2. **Phase 2**: Copy/Move/Paste operations
3. **Phase 3**: Upload dengan progress (SSE/WebSocket)
4. **Phase 4**: Compress & Extract dengan progress
5. **Phase 5**: Performance optimization & testing

---

## ✅ Success Criteria

- [ ] Semua CRUD operations berfungsi
- [ ] Copy/Paste works untuk files dan folders
- [ ] Upload menampilkan progress real-time (percentage)
- [ ] Compress ke ZIP berfungsi dengan progress
- [ ] Extract ZIP berfungsi dengan progress
- [ ] Response time < 100ms untuk operasi kecil
- [ ] Dapat handle file > 1GB tanpa memory issues
- [ ] Concurrent uploads tanpa race condition
