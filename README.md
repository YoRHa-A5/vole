# Vole

A stateless HTTP service that extracts clean Markdown from web pages.

## API

### POST /extract

```json
POST /extract
Content-Type: application/json

{ "url": "https://example.com/article", "format": "markdown" }
```

### GET /extract

```
GET /extract?url=https://example.com/article&format=markdown
```

### Response

```json
{
  "url": "https://example.com/article",
  "title": "Article Title",
  "description": "Meta description from the page",
  "content": "# Title\n\nParagraph content...",
  "content_type": "text/markdown",
  "status_code": 200,
  "extraction_method": "readability",
  "language": "en"
}
```

### Error Responses

| Error | HTTP Status | Response |
|-------|-------------|----------|
| `fetch_failed` | 502 | `{ "error": "fetch_failed", "detail": "..." }` |
| `upstream_error` | 4xx/5xx | `{ "error": "upstream_error", "status_code": 404 }` |
| `unsupported_content_type` | 415 | `{ "error": "unsupported_content_type", "content_type": "application/pdf" }` |
| `parse_error` | 500 | `{ "error": "parse_error", "detail": "..." }` |
| `invalid_url` | 400 | `{ "error": "invalid_url", "detail": "only http and https URLs are supported" }` |

## How It Works

1. HTTP GET the URL (Go stdlib, with connect and read timeouts)
2. Check content type — accept `text/html` and `text/plain`, reject everything else (415)
3. Try **Mozilla Readability** to extract the main article content
4. Convert extracted HTML → Markdown (GitHub Flavored)
5. If Readability finds nothing meaningful, fall back to converting the entire `<body>`
6. If that yields nothing, return a warning with empty content

## Configuration

Copy `.env.example` to `.env` and adjust the variables if needed:

```bash
cp .env.example .env
```

| Variable | Default | Description |
|----------|---------|-------------|
| `LISTEN_ADDR` | `:7007` | HTTP listen address |
| `READ_TIMEOUT` | `30s` | Max time to read the target page |
| `CONNECT_TIMEOUT` | `15s` | Max time to establish a connection |
| `MAX_BODY_SIZE` | `10485760` | Max HTML body size in bytes (10 MB) |
| `USER_AGENT` | `Go-http-client/1.1` | User-Agent sent with requests |


Environment variables take precedence over `.env` values.

## Build

```bash
# Development
go build -o bin/vole .

# Cross-compile for Linux/ARM64
GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o bin/vole .
```

## Run

```bash
bin/vole
# or with custom config:
LISTEN_ADDR=:7007 READ_TIMEOUT=30s bin/vole
```

## Architecture

```
  Client  ──HTTP──→  vole (Server, :7007)
  ←───────────  Markdown
                          │
                   Target URL (Internet)
```

### Dependencies

| Library | Purpose |
|---------|---------|
| `go-shiori/go-readability` | Content extraction (Mozilla Readability port) |
| `JohannesKaufmann/html-to-markdown` | HTML → Markdown conversion |
| `golang.org/x/net/html` | HTML parsing for metadata extraction |

### What Vole Does NOT Do

- Execute JavaScript (use CamoFox for that)
- Crawl / follow links
- Handle sessions, cookies, or CAPTCHAs
- Cache responses
- Rate limit requests
