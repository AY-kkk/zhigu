package futures

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"runtime/debug"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/ledongthuc/pdf"
)

var (
	futuresExtractionSlots  = make(chan struct{}, 2)
	futuresExtractionBinary = func() (string, error) { return os.Executable() }
)

type futuresExtractionRequest struct {
	Filename string `json:"filename"`
	MIME     string `json:"mime"`
	Content  []byte `json:"content"`
}

const (
	maxFuturesDocumentBytes = 20 * 1024 * 1024
	maxFuturesDocumentPages = 100
	maxFuturesDocumentRunes = 120_000
	maxDOCXEntries          = 256
	maxDOCXUncompressed     = 120 * 1024 * 1024
)

type DocumentExtraction struct {
	ID             string  `json:"id"`
	Status         string  `json:"status"`
	MIME           string  `json:"mime"`
	Bytes          int     `json:"bytes"`
	PageCount      *int    `json:"page_count"`
	CharacterCount *int    `json:"character_count"`
	FailureCode    *string `json:"failure_code"`
	Text           string  `json:"-"`
}

func ExtractFuturesDocument(filename, declaredMIME string, content []byte) (DocumentExtraction, error) {
	if len(content) == 0 || len(content) > maxFuturesDocumentBytes {
		return DocumentExtraction{}, ErrDocumentLimit
	}
	mime, err := futuresDocumentMIME(filename, declaredMIME)
	if err != nil {
		return DocumentExtraction{}, err
	}
	result := DocumentExtraction{Status: "ready", MIME: mime, Bytes: len(content)}
	switch mime {
	case "text/plain":
		text, err := extractFuturesTXT(content)
		if err != nil {
			return DocumentExtraction{}, err
		}
		result.Text = text
	case "application/pdf":
		text, pages, err := extractFuturesPDF(content)
		if err != nil {
			return DocumentExtraction{}, err
		}
		result.Text, result.PageCount = text, &pages
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		text, err := extractFuturesDOCX(content)
		if err != nil {
			return DocumentExtraction{}, err
		}
		result.Text = text
	default:
		return DocumentExtraction{}, ErrDocumentUnreadable
	}
	count := utf8.RuneCountInString(result.Text)
	if count == 0 || count > maxFuturesDocumentRunes {
		return DocumentExtraction{}, ErrDocumentLimit
	}
	result.CharacterCount = &count
	return result, nil
}

func ExtractFuturesDocumentIsolated(ctx context.Context, filename, declaredMIME string, content []byte) (DocumentExtraction, error) {
	select {
	case futuresExtractionSlots <- struct{}{}:
		defer func() { <-futuresExtractionSlots }()
	case <-ctx.Done():
		return DocumentExtraction{}, ErrDocumentTimeout
	}
	execCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	binary, err := futuresExtractionBinary()
	if err != nil {
		return DocumentExtraction{}, ErrDocumentUnreadable
	}
	payload, err := json.Marshal(futuresExtractionRequest{Filename: filename, MIME: declaredMIME, Content: content})
	if err != nil {
		return DocumentExtraction{}, ErrDocumentUnreadable
	}
	command := exec.CommandContext(execCtx, binary, "--futures-extract-v1")
	command.Stdin = bytes.NewReader(payload)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		if errors.Is(execCtx.Err(), context.DeadlineExceeded) || errors.Is(execCtx.Err(), context.Canceled) {
			return DocumentExtraction{}, ErrDocumentTimeout
		}
		return DocumentExtraction{}, ErrDocumentUnreadable
	}
	var result DocumentExtraction
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return DocumentExtraction{}, ErrDocumentUnreadable
	}
	return result, nil
}

func RunFuturesExtractionCLI() int {
	setFuturesExtractionProcessLimits()
	raw, err := io.ReadAll(io.LimitReader(os.Stdin, maxFuturesDocumentBytes*2+4096))
	if err != nil {
		return 2
	}
	var request futuresExtractionRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		return 2
	}
	result, err := ExtractFuturesDocument(request.Filename, request.MIME, request.Content)
	if err != nil {
		return 3
	}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		return 4
	}
	return 0
}

func setFuturesExtractionProcessLimits() {
	debug.SetMemoryLimit(512 * 1024 * 1024)
	setRlimit(syscall.RLIMIT_CPU, 30)
	setRlimit(syscall.RLIMIT_FSIZE, maxFuturesDocumentBytes)
	setRlimit(syscall.RLIMIT_DATA, 512*1024*1024)
	setRlimit(syscall.RLIMIT_NOFILE, 64)
}

func setRlimit(resource int, value uint64) {
	limit := &syscall.Rlimit{Cur: value, Max: value}
	_ = syscall.Setrlimit(resource, limit)
}

func futuresDocumentMIME(filename, declared string) (string, error) {
	ext := strings.ToLower(path.Ext(filename))
	mime := ""
	switch ext {
	case ".pdf":
		mime = "application/pdf"
	case ".docx":
		mime = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".txt":
		mime = "text/plain"
	default:
		return "", ErrDocumentUnreadable
	}
	declared = strings.ToLower(strings.TrimSpace(strings.SplitN(declared, ";", 2)[0]))
	if declared != "" && declared != mime && !(mime == "text/plain" && strings.HasPrefix(declared, "text/")) {
		return "", ErrDocumentUnreadable
	}
	return mime, nil
}

func extractFuturesTXT(content []byte) (string, error) {
	if bytes.IndexByte(content, 0) >= 0 || !utf8.Valid(content) {
		return "", ErrDocumentUnreadable
	}
	text := strings.TrimSpace(strings.TrimPrefix(string(content), "\ufeff"))
	if text == "" {
		return "", ErrDocumentUnreadable
	}
	return text, nil
}

func extractFuturesPDF(content []byte) (string, int, error) {
	if !bytes.HasPrefix(content, []byte("%PDF-")) {
		return "", 0, ErrDocumentUnreadable
	}
	reader, err := pdf.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return "", 0, ErrDocumentUnreadable
	}
	pages := reader.NumPage()
	if pages < 1 || pages > maxFuturesDocumentPages {
		return "", pages, ErrDocumentLimit
	}
	var text strings.Builder
	for index := 1; index <= pages; index++ {
		page := reader.Page(index)
		if page.V.IsNull() {
			continue
		}
		value, err := page.GetPlainText(nil)
		if err != nil {
			continue
		}
		text.WriteString(value)
		text.WriteByte('\n')
		if utf8.RuneCountInString(text.String()) > maxFuturesDocumentRunes {
			return "", pages, ErrDocumentLimit
		}
	}
	out := strings.TrimSpace(text.String())
	if out == "" {
		return "", pages, ErrDocumentUnreadable
	}
	return out, pages, nil
}

func extractFuturesDOCX(content []byte) (string, error) {
	reader, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return "", ErrDocumentUnreadable
	}
	if len(reader.File) > maxDOCXEntries {
		return "", ErrDocumentLimit
	}
	var total uint64
	var documentXML []byte
	for _, file := range reader.File {
		name := strings.ReplaceAll(file.Name, "\\", "/")
		clean := path.Clean(name)
		if clean == "." || strings.HasPrefix(clean, "../") || path.IsAbs(clean) || clean != name {
			return "", ErrDocumentUnreadable
		}
		lower := strings.ToLower(clean)
		if strings.Contains(lower, "vbaproject") || strings.HasSuffix(lower, ".bin") || strings.HasSuffix(lower, ".exe") {
			return "", ErrDocumentUnreadable
		}
		total += file.UncompressedSize64
		if total > maxDOCXUncompressed {
			return "", ErrDocumentLimit
		}
		if file.UncompressedSize64 > 1024*1024 && file.CompressedSize64 > 0 && file.UncompressedSize64/file.CompressedSize64 > 200 {
			return "", ErrDocumentLimit
		}
		if lower == "word/document.xml" {
			documentXML, err = readBoundedZIPFile(file, maxDOCXUncompressed)
			if err != nil {
				return "", err
			}
		}
		if strings.HasSuffix(lower, ".rels") {
			raw, readErr := readBoundedZIPFile(file, 2*1024*1024)
			if readErr != nil {
				return "", readErr
			}
			if strings.Contains(strings.ToLower(string(raw)), `targetmode="external"`) {
				return "", ErrDocumentUnreadable
			}
		}
	}
	if len(documentXML) == 0 {
		return "", ErrDocumentUnreadable
	}
	text, err := docxPlainText(documentXML)
	if err != nil {
		return "", ErrDocumentUnreadable
	}
	return text, nil
}

func readBoundedZIPFile(file *zip.File, limit uint64) ([]byte, error) {
	if file.UncompressedSize64 > limit {
		return nil, ErrDocumentLimit
	}
	reader, err := file.Open()
	if err != nil {
		return nil, ErrDocumentUnreadable
	}
	defer reader.Close()
	raw, err := io.ReadAll(io.LimitReader(reader, int64(limit)+1))
	if err != nil || uint64(len(raw)) > limit {
		return nil, ErrDocumentLimit
	}
	return raw, nil
}

func docxPlainText(raw []byte) (string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(raw))
	var text strings.Builder
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		switch value := token.(type) {
		case xml.StartElement:
			if value.Name.Local == "t" {
				var content string
				if err := decoder.DecodeElement(&content, &value); err != nil {
					return "", err
				}
				text.WriteString(content)
			}
		case xml.CharData:
			text.Write(value)
		}
	}
	out := strings.TrimSpace(text.String())
	if out == "" {
		return "", fmt.Errorf("empty docx")
	}
	return out, nil
}
