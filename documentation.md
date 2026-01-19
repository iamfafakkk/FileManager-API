# FileManager API Documentation

See [README.md](./README.md) for full documentation.

## Quick Start

```bash
# Run the server
go run cmd/main.go

# Or run the compiled binary
./filemanager-api
```

## Authentication

All requests require headers:
- `X-API-Key: filemanager-secret-key`
- `X-User-Site: {username}`

Example:
```bash
curl http://localhost:3000/api/v1/files \
  -H "X-API-Key: filemanager-secret-key" \
  -H "X-User-Site: myuser"
```
