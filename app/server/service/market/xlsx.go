package market

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"strings"
)

type xWorksheet struct {
	SheetData struct {
		Rows []xRow `xml:"row"`
	} `xml:"sheetData"`
}

type xRow struct {
	Cells []xCell `xml:"c"`
}

type xCell struct {
	Ref string `xml:"r,attr"`
	T   string `xml:"t,attr"`
	V   string `xml:"v"`
	Is  xInner `xml:"is"`
}

type xInner struct {
	T []string `xml:"t"`
	R []xRun   `xml:"r"`
}

type xRun struct {
	T string `xml:"t"`
}

type xSST struct {
	SI []xSI `xml:"si"`
}

type xSI struct {
	T string `xml:"t"`
	R []xRun `xml:"r"`
}

func readXLSXGrid(raw []byte) ([][]string, error) {
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, err
	}
	ss := map[int]string{}
	var sheetXML []byte
	for _, f := range zr.File {
		name := strings.ReplaceAll(f.Name, "\\", "/")
		switch {
		case name == "xl/sharedStrings.xml":
			body, rerr := readZipFile(f)
			if rerr != nil {
				return nil, rerr
			}
			var sst xSST
			if xml.Unmarshal(body, &sst) != nil {
				continue
			}
			for i, si := range sst.SI {
				ss[i] = joinRuns(si.T, si.R)
			}
		case strings.HasPrefix(name, "xl/worksheets/sheet") && strings.HasSuffix(name, ".xml") && sheetXML == nil:
			sheetXML, err = readZipFile(f)
			if err != nil {
				return nil, err
			}
		}
	}
	if len(sheetXML) == 0 {
		return nil, jsonError{"xlsx 无工作表"}
	}
	var ws xWorksheet
	if xml.Unmarshal(sheetXML, &ws) != nil {
		return nil, jsonError{"xlsx 工作表无法解析"}
	}
	grid := make([][]string, 0, len(ws.SheetData.Rows))
	for _, row := range ws.SheetData.Rows {
		var line []string
		for _, c := range row.Cells {
			col := colIndex(c.Ref)
			if col < 0 {
				continue
			}
			if col >= len(line) {
				grow := make([]string, col+1)
				copy(grow, line)
				line = grow
			}
			line[col] = cellText(c, ss)
		}
		grid = append(grid, line)
	}
	return grid, nil
}

func readZipFile(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(io.LimitReader(rc, 12<<20))
}

func joinRuns(t string, runs []xRun) string {
	if t != "" && len(runs) == 0 {
		return t
	}
	var b strings.Builder
	b.WriteString(t)
	for _, r := range runs {
		b.WriteString(r.T)
	}
	return b.String()
}

func cellText(c xCell, ss map[int]string) string {
	switch c.T {
	case "s":
		if s, ok := ss[atoiSafe(strings.TrimSpace(c.V))]; ok {
			return strings.TrimSpace(s)
		}
		return ""
	case "inlineStr", "str":
		parts := append([]string{}, c.Is.T...)
		for _, r := range c.Is.R {
			parts = append(parts, r.T)
		}
		if len(parts) > 0 {
			return strings.TrimSpace(strings.Join(parts, ""))
		}
		return strings.TrimSpace(c.V)
	default:
		if len(c.Is.T) > 0 || len(c.Is.R) > 0 {
			return strings.TrimSpace(joinRuns(strings.Join(c.Is.T, ""), c.Is.R))
		}
		return strings.TrimSpace(c.V)
	}
}

func colIndex(ref string) int {
	n := 0
	for _, r := range ref {
		if r < 'A' || r > 'Z' {
			break
		}
		n = n*26 + int(r-'A'+1)
	}
	if n == 0 {
		return -1
	}
	return n - 1
}

func atoiSafe(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return n
		}
		n = n*10 + int(r-'0')
	}
	return n
}

func headerCols(grid [][]string, keys ...string) (map[string]int, int) {
	want := map[string]struct{}{}
	for _, k := range keys {
		want[k] = struct{}{}
	}
	for i, row := range grid {
		if i > 12 {
			break
		}
		idx := map[string]int{}
		for c, v := range row {
			v = strings.TrimSpace(strings.ReplaceAll(v, "\n", " "))
			if _, ok := want[v]; ok {
				idx[v] = c
			}
		}
		if len(idx) == len(want) {
			return idx, i
		}
	}
	return nil, -1
}

func cellAt(row []string, col int) string {
	if col < 0 || col >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[col])
}
