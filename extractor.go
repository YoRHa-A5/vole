package main

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	md "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/JohannesKaufmann/html-to-markdown/plugin"
	readability "github.com/go-shiori/go-readability"
	"golang.org/x/net/html"
)

// ExtractResult holds the extracted content and metadata.
type ExtractResult struct {
	URL              string `json:"url"`
	Title            string `json:"title"`
	Description      string `json:"description"`
	Content          string `json:"content"`
	ContentType      string `json:"content_type"`
	StatusCode       int    `json:"status_code"`
	ExtractionMethod string `json:"extraction_method"`
	Language         string `json:"language,omitempty"`
	Warning          string `json:"warning,omitempty"`
}

// ExtractError holds error information for error responses.
type ExtractError struct {
	Error      string `json:"error"`
	Detail     string `json:"detail,omitempty"`
	StatusCode *int   `json:"status_code,omitempty"`
	CT         string `json:"content_type,omitempty"`
}

// extractClient is the HTTP client used for fetching URLs.
type extractClient struct {
	httpClient  *http.Client
	maxBodySize int64
	userAgent   string
	converter   *md.Converter
}

// newExtractClient creates a new HTTP client with configured timeouts and a reusable markdown converter.
func newExtractClient(connectTimeout, readTimeout time.Duration, maxBodySize int64, userAgent string) *extractClient {
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: connectTimeout,
		}).DialContext,
		MaxIdleConns:        10,
		IdleConnTimeout:     30 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}
	httpClient := &http.Client{
		Transport: transport,
		Timeout:   readTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}
	converter := md.NewConverter("", true, nil)
	converter.Use(plugin.GitHubFlavored())
	return &extractClient{
		httpClient:  httpClient,
		maxBodySize: maxBodySize,
		userAgent:   userAgent,
		converter:   converter,
	}
}

// fetch retrieves the content from the given URL.
func (c *extractClient) fetch(rawURL string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch failed: %w", err)
	}
	return resp, nil
}

// extract performs the full extraction pipeline: fetch → parse → readability → markdown.
// Returns (*ExtractResult, nil) on success, (*ExtractError, nil) on client errors (415, 502),
// or (nil, error) for internal failures (parse errors, etc.).
func (c *extractClient) extract(rawURL string) (any, error) {
	resp, err := c.fetch(rawURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	statusCode := resp.StatusCode

	// Check for non-2xx status codes
	if statusCode < 200 || statusCode >= 300 {
		return &ExtractError{
			Error:      "upstream_error",
			Detail:     fmt.Sprintf("upstream returned status %d", statusCode),
			StatusCode: &statusCode,
		}, nil
	}

	// Check content type
	contentType := resp.Header.Get("Content-Type")
	if ct := parseContentType(contentType); ct == "html" {
		return c.extractHTML(rawURL, resp.Body, statusCode)
	} else if ct == "plain" {
		return c.extractPlain(rawURL, resp.Body, statusCode)
	}
	// Everything else: 415
	return &ExtractError{
		Error:   "unsupported_content_type",
		Detail:  contentType,
		CT:      contentType,
	}, nil
}

// extractHTML handles HTML content through the readability → raw fallback pipeline.
func (c *extractClient) extractHTML(rawURL string, body io.Reader, statusCode int) (*ExtractResult, error) {
	bodyBytes, err := io.ReadAll(io.LimitReader(body, c.maxBodySize))
	if err != nil {
		return nil, fmt.Errorf("failed to read body: %w", err)
	}
	bodyStr := string(bodyBytes)

	// Parse HTML for metadata
	doc, err := html.Parse(strings.NewReader(bodyStr))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}
	lang := getLangAttribute(doc)
	metaDesc := getMetaDescription(doc)

	// Try readability first
	parsedURL, _ := url.Parse(rawURL)
	article, err := readability.FromReader(bytes.NewReader(bodyBytes), parsedURL)
	if err == nil && len(strings.TrimSpace(article.Content)) > 0 {
		mdContent, convErr := c.convertToMarkdown(article.Content)
		if convErr == nil && len(strings.TrimSpace(mdContent)) >= 50 {
			return &ExtractResult{
				URL:              rawURL,
				Title:            article.Title,
				Description:      metaDesc,
				Content:          mdContent,
				ContentType:      "text/markdown",
				StatusCode:       statusCode,
				ExtractionMethod: "readability",
				Language:         lang,
			}, nil
		}
		// Readability content too short, fall through to raw
	}

	// Fallback: raw body → markdown
	markdown, err := c.convertToMarkdown(bodyStr)
	if err != nil {
		return nil, fmt.Errorf("failed to convert raw HTML to markdown: %w", err)
	}

	title := extractTitle(doc)
	desc := metaDesc // prefer meta description over readability excerpt for raw fallback

	if len(strings.TrimSpace(markdown)) == 0 {
		return &ExtractResult{
			URL:              rawURL,
			Title:            title,
			Description:      desc,
			Content:          "",
			ContentType:      "text/markdown",
			StatusCode:       statusCode,
			ExtractionMethod: "raw",
			Language:         lang,
			Warning:          "no content extracted",
		}, nil
	}

	return &ExtractResult{
		URL:              rawURL,
		Title:            title,
		Description:      desc,
		Content:          markdown,
		ContentType:      "text/markdown",
		StatusCode:       statusCode,
		ExtractionMethod: "raw",
		Language:         lang,
	}, nil
}

// extractPlain handles plain text content as pass-through.
func (c *extractClient) extractPlain(rawURL string, body io.Reader, statusCode int) (*ExtractResult, error) {
	text, err := io.ReadAll(io.LimitReader(body, c.maxBodySize))
	if err != nil {
		return nil, fmt.Errorf("failed to read plain text: %w", err)
	}
	return &ExtractResult{
		URL:              rawURL,
		Title:            "",
		Description:      "",
		Content:          string(text),
		ContentType:      "text/plain",
		StatusCode:       statusCode,
		ExtractionMethod: "plain_text",
	}, nil
}

// parseContentType classifies the content type.
func parseContentType(contentType string) string {
	ct := strings.ToLower(contentType)
	if strings.Contains(ct, "text/html") || strings.Contains(ct, "application/xhtml") {
		return "html"
	}
	if strings.HasPrefix(ct, "text/plain") {
		return "plain"
	}
	return "other"
}

// convertToMarkdown converts HTML string to Markdown using the pre-initialized converter.
func (c *extractClient) convertToMarkdown(htmlStr string) (string, error) {
	markdown, err := c.converter.ConvertString(htmlStr)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(markdown), nil
}

// extractTitle extracts the title from an already-parsed HTML document.
func extractTitle(doc *html.Node) string {
	var title string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "title" {
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.TextNode {
					title = strings.TrimSpace(c.Data)
					return
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
			if title != "" {
				return
			}
		}
	}
	walk(doc)
	return title
}

// getLangAttribute extracts the lang attribute from the HTML document.
func getLangAttribute(doc *html.Node) string {
	var lang string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			for _, a := range n.Attr {
				if a.Key == "lang" || a.Key == "xml:lang" {
					lang = a.Val
					return
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
			if lang != "" {
				return
			}
		}
	}
	walk(doc)
	return lang
}

// getMetaDescription extracts the content of the meta description tag.
func getMetaDescription(doc *html.Node) string {
	var desc string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "meta" {
			for _, a := range n.Attr {
				if a.Key == "name" && strings.EqualFold(a.Val, "description") {
					for _, a2 := range n.Attr {
						if a2.Key == "content" {
							desc = strings.TrimSpace(a2.Val)
							return
						}
					}
					return
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
			if desc != "" {
				return
			}
		}
	}
	walk(doc)
	return desc
}
