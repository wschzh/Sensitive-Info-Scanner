package extract

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

// maxDOCXUncompressed docx 解压后大小上限（字节），防止 zip 炸弹或超大文档拖垮进程。
const maxDOCXUncompressed = 50 * 1024 * 1024 // 50MB

// DOCX 从 docx 提取正文文本：解压 word/document.xml，收集 <w:t> 文本，
// <w:tab> 转 \t，<w:br>/<w:cr> 与段落结束 <w:p> 转 \n。
func DOCX(path string) (string, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return "", err
	}
	defer zr.Close()

	var total uint64
	for _, f := range zr.File {
		total += f.UncompressedSize64
		if total > maxDOCXUncompressed {
			return "", fmt.Errorf("docx 解压后约 %dMB 超过 %dMB 上限，跳过以防 OOM",
				total/1024/1024, maxDOCXUncompressed/1024/1024)
		}
	}

	var docFile *zip.File
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			docFile = f
			break
		}
	}
	if docFile == nil {
		return "", fmt.Errorf("docx 内未找到 word/document.xml")
	}
	rc, err := docFile.Open()
	if err != nil {
		return "", err
	}
	defer rc.Close()
	return parseDocxXML(rc)
}

func parseDocxXML(r io.Reader) (string, error) {
	var b strings.Builder
	dec := xml.NewDecoder(r)
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return b.String(), err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "t":
				if txt, err := readCharData(dec); err == nil {
					b.WriteString(txt)
				}
			case "tab":
				b.WriteByte('\t')
			case "br", "cr":
				b.WriteByte('\n')
			}
		case xml.EndElement:
			if t.Name.Local == "p" {
				b.WriteByte('\n')
			}
		}
	}
	return b.String(), nil
}

// readCharData 读取当前元素的字符数据直到结束标签。
func readCharData(dec *xml.Decoder) (string, error) {
	var b strings.Builder
	for {
		tok, err := dec.Token()
		if err != nil {
			return b.String(), err
		}
		switch t := tok.(type) {
		case xml.CharData:
			b.Write(t)
		case xml.EndElement:
			return b.String(), nil
		}
	}
}
