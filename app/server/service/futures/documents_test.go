package futures

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

func zipDOCX(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, content := range files {
		file, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestFuturesDocumentValidationAndExtractionBoundaries(t *testing.T) {
	txt, err := ExtractFuturesDocument("note.txt", "text/plain", []byte("这是一个有效的研究材料文本，包含二十个以上字符用于解析。"))
	if err != nil || !strings.Contains(txt.Text, "研究材料") || txt.Status != "ready" {
		t.Fatalf("txt=%+v err=%v", txt, err)
	}
	docx := zipDOCX(t, map[string]string{
		"[Content_Types].xml": "<Types/>",
		"word/document.xml":   `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>DOCX研究材料</w:t></w:r></w:p></w:body></w:document>`,
	})
	got, err := ExtractFuturesDocument("report.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", docx)
	if err != nil || !strings.Contains(got.Text, "DOCX研究材料") {
		t.Fatalf("docx=%+v err=%v", got, err)
	}
	for name, content := range map[string][]byte{
		"bad-text":    []byte{0xff, 0xfe, 0x00, 0x01},
		"bad-pdf":     []byte("%PDF-not-a-real-document"),
		"oversize":    make([]byte, 20*1024*1024+1),
		"wrong-magic": []byte("this is not a pdf"),
	} {
		t.Run(name, func(t *testing.T) {
			filename, media := "input.txt", "text/plain"
			if name == "bad-pdf" || name == "wrong-magic" {
				filename, media = "input.pdf", "application/pdf"
			}
			if _, err := ExtractFuturesDocument(filename, media, content); err == nil {
				t.Fatal("expected document rejection")
			}
		})
	}
}

func TestFuturesDOCXRejectsTraversalAndZipBomb(t *testing.T) {
	traversal := zipDOCX(t, map[string]string{
		"word/document.xml": "<w:document/>",
		"../escape":         "bad",
	})
	if _, err := ExtractFuturesDocument("bad.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", traversal); err == nil {
		t.Fatal("path traversal must be rejected")
	}
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	file, _ := writer.Create("word/document.xml")
	_, _ = file.Write([]byte(strings.Repeat("A", 2*1024*1024)))
	_ = writer.Close()
	if _, err := ExtractFuturesDocument("bomb.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", buffer.Bytes()); err == nil {
		t.Fatal("compression-ratio bomb must be rejected")
	}
}
