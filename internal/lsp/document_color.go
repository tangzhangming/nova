package lsp

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"go.lsp.dev/protocol"
)

// Color 颜色
type Color struct {
	Red   float64 `json:"red"`
	Green float64 `json:"green"`
	Blue  float64 `json:"blue"`
	Alpha float64 `json:"alpha"`
}

// ColorInformation 颜色信息
type ColorInformation struct {
	Range protocol.Range `json:"range"`
	Color Color          `json:"color"`
}

// ColorPresentation 颜色表示
type ColorPresentation struct {
	Label    string                `json:"label"`
	TextEdit *protocol.TextEdit    `json:"textEdit,omitempty"`
	AdditionalTextEdits []protocol.TextEdit `json:"additionalTextEdits,omitempty"`
}

// DocumentColorParams 文档颜色参数
type DocumentColorParams struct {
	TextDocument protocol.TextDocumentIdentifier `json:"textDocument"`
}

// ColorPresentationParams 颜色表示参数
type ColorPresentationParams struct {
	TextDocument protocol.TextDocumentIdentifier `json:"textDocument"`
	Color        Color                           `json:"color"`
	Range        protocol.Range                  `json:"range"`
}

// handleDocumentColor 处理文档颜色请求
func (s *Server) handleDocumentColor(id json.RawMessage, params json.RawMessage) {
	var p DocumentColorParams
	if err := json.Unmarshal(params, &p); err != nil {
		s.sendError(id, -32700, "Parse error")
		return
	}

	docURI := string(p.TextDocument.URI)
	doc := s.documents.Get(docURI)
	if doc == nil {
		s.sendResult(id, []ColorInformation{})
		return
	}

	colors := s.findDocumentColors(doc)
	s.sendResult(id, colors)
}

// handleColorPresentation 处理颜色表示请求
func (s *Server) handleColorPresentation(id json.RawMessage, params json.RawMessage) {
	var p ColorPresentationParams
	if err := json.Unmarshal(params, &p); err != nil {
		s.sendError(id, -32700, "Parse error")
		return
	}

	presentations := s.getColorPresentations(p.Color, p.Range)
	s.sendResult(id, presentations)
}

// findDocumentColors 查找文档中的颜色
func (s *Server) findDocumentColors(doc *Document) []ColorInformation {
	var colors []ColorInformation

	// 正则表达式匹配各种颜色格式
	hexPattern := regexp.MustCompile(`#([0-9a-fA-F]{3}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})`)
	rgbPattern := regexp.MustCompile(`rgb\s*\(\s*(\d{1,3})\s*,\s*(\d{1,3})\s*,\s*(\d{1,3})\s*\)`)
	rgbaPattern := regexp.MustCompile(`rgba\s*\(\s*(\d{1,3})\s*,\s*(\d{1,3})\s*,\s*(\d{1,3})\s*,\s*([0-9.]+)\s*\)`)
	hslPattern := regexp.MustCompile(`hsl\s*\(\s*(\d{1,3})\s*,\s*(\d{1,3})%\s*,\s*(\d{1,3})%\s*\)`)

	for lineNum, lineText := range doc.Lines {
		// 检查十六进制颜色
		matches := hexPattern.FindAllStringSubmatchIndex(lineText, -1)
		for _, match := range matches {
			if len(match) >= 4 {
				start := match[0]
				end := match[1]
				hexValue := lineText[match[2]:match[3]]
				
				color := parseHexColor(hexValue)
				if color != nil {
					colors = append(colors, ColorInformation{
						Range: protocol.Range{
							Start: protocol.Position{Line: uint32(lineNum), Character: uint32(start)},
							End:   protocol.Position{Line: uint32(lineNum), Character: uint32(end)},
						},
						Color: *color,
					})
				}
			}
		}

		// 检查 rgb() 颜色
		rgbMatches := rgbPattern.FindAllStringSubmatchIndex(lineText, -1)
		for _, match := range rgbMatches {
			if len(match) >= 8 {
				start := match[0]
				end := match[1]
				r := lineText[match[2]:match[3]]
				g := lineText[match[4]:match[5]]
				b := lineText[match[6]:match[7]]
				
				color := parseRGBColor(r, g, b, "1")
				if color != nil {
					colors = append(colors, ColorInformation{
						Range: protocol.Range{
							Start: protocol.Position{Line: uint32(lineNum), Character: uint32(start)},
							End:   protocol.Position{Line: uint32(lineNum), Character: uint32(end)},
						},
						Color: *color,
					})
				}
			}
		}

		// 检查 rgba() 颜色
		rgbaMatches := rgbaPattern.FindAllStringSubmatchIndex(lineText, -1)
		for _, match := range rgbaMatches {
			if len(match) >= 10 {
				start := match[0]
				end := match[1]
				r := lineText[match[2]:match[3]]
				g := lineText[match[4]:match[5]]
				b := lineText[match[6]:match[7]]
				a := lineText[match[8]:match[9]]
				
				color := parseRGBColor(r, g, b, a)
				if color != nil {
					colors = append(colors, ColorInformation{
						Range: protocol.Range{
							Start: protocol.Position{Line: uint32(lineNum), Character: uint32(start)},
							End:   protocol.Position{Line: uint32(lineNum), Character: uint32(end)},
						},
						Color: *color,
					})
				}
			}
		}

		// 检查 hsl() 颜色
		hslMatches := hslPattern.FindAllStringSubmatchIndex(lineText, -1)
		for _, match := range hslMatches {
			if len(match) >= 8 {
				start := match[0]
				end := match[1]
				h := lineText[match[2]:match[3]]
				s := lineText[match[4]:match[5]]
				l := lineText[match[6]:match[7]]
				
				color := parseHSLColor(h, s, l)
				if color != nil {
					colors = append(colors, ColorInformation{
						Range: protocol.Range{
							Start: protocol.Position{Line: uint32(lineNum), Character: uint32(start)},
							End:   protocol.Position{Line: uint32(lineNum), Character: uint32(end)},
						},
						Color: *color,
					})
				}
			}
		}

		// 检查命名颜色
		namedColors := map[string]Color{
			"red":     {Red: 1, Green: 0, Blue: 0, Alpha: 1},
			"green":   {Red: 0, Green: 0.5, Blue: 0, Alpha: 1},
			"blue":    {Red: 0, Green: 0, Blue: 1, Alpha: 1},
			"white":   {Red: 1, Green: 1, Blue: 1, Alpha: 1},
			"black":   {Red: 0, Green: 0, Blue: 0, Alpha: 1},
			"yellow":  {Red: 1, Green: 1, Blue: 0, Alpha: 1},
			"cyan":    {Red: 0, Green: 1, Blue: 1, Alpha: 1},
			"magenta": {Red: 1, Green: 0, Blue: 1, Alpha: 1},
			"orange":  {Red: 1, Green: 0.647, Blue: 0, Alpha: 1},
			"purple":  {Red: 0.5, Green: 0, Blue: 0.5, Alpha: 1},
			"pink":    {Red: 1, Green: 0.753, Blue: 0.796, Alpha: 1},
			"gray":    {Red: 0.5, Green: 0.5, Blue: 0.5, Alpha: 1},
			"grey":    {Red: 0.5, Green: 0.5, Blue: 0.5, Alpha: 1},
		}

		for name, color := range namedColors {
			// 在字符串字面量中查找颜色名
			pattern := regexp.MustCompile(`["']` + name + `["']`)
			matches := pattern.FindAllStringIndex(lineText, -1)
			for _, match := range matches {
				colors = append(colors, ColorInformation{
					Range: protocol.Range{
						Start: protocol.Position{Line: uint32(lineNum), Character: uint32(match[0] + 1)},
						End:   protocol.Position{Line: uint32(lineNum), Character: uint32(match[1] - 1)},
					},
					Color: color,
				})
			}
		}
	}

	return colors
}

// getColorPresentations 获取颜色表示
func (s *Server) getColorPresentations(color Color, rang protocol.Range) []ColorPresentation {
	var presentations []ColorPresentation

	r := int(color.Red * 255)
	g := int(color.Green * 255)
	b := int(color.Blue * 255)

	// 十六进制格式
	hexColor := "#" + toHex(r) + toHex(g) + toHex(b)
	presentations = append(presentations, ColorPresentation{
		Label: hexColor,
		TextEdit: &protocol.TextEdit{
			Range:   rang,
			NewText: hexColor,
		},
	})

	// 带 alpha 的十六进制格式
	if color.Alpha < 1 {
		a := int(color.Alpha * 255)
		hexColorA := "#" + toHex(r) + toHex(g) + toHex(b) + toHex(a)
		presentations = append(presentations, ColorPresentation{
			Label: hexColorA,
			TextEdit: &protocol.TextEdit{
				Range:   rang,
				NewText: hexColorA,
			},
		})
	}

	// RGB 格式
	rgbColor := "rgb(" + strconv.Itoa(r) + ", " + strconv.Itoa(g) + ", " + strconv.Itoa(b) + ")"
	presentations = append(presentations, ColorPresentation{
		Label: rgbColor,
		TextEdit: &protocol.TextEdit{
			Range:   rang,
			NewText: rgbColor,
		},
	})

	// RGBA 格式
	rgbaColor := "rgba(" + strconv.Itoa(r) + ", " + strconv.Itoa(g) + ", " + strconv.Itoa(b) + ", " + formatFloat(color.Alpha) + ")"
	presentations = append(presentations, ColorPresentation{
		Label: rgbaColor,
		TextEdit: &protocol.TextEdit{
			Range:   rang,
			NewText: rgbaColor,
		},
	})

	return presentations
}

// parseHexColor 解析十六进制颜色
func parseHexColor(hex string) *Color {
	hex = strings.TrimPrefix(hex, "#")

	var r, g, b, a float64
	a = 1.0

	switch len(hex) {
	case 3:
		// #RGB
		rVal, _ := strconv.ParseInt(string(hex[0])+string(hex[0]), 16, 0)
		gVal, _ := strconv.ParseInt(string(hex[1])+string(hex[1]), 16, 0)
		bVal, _ := strconv.ParseInt(string(hex[2])+string(hex[2]), 16, 0)
		r = float64(rVal) / 255
		g = float64(gVal) / 255
		b = float64(bVal) / 255
	case 6:
		// #RRGGBB
		rVal, _ := strconv.ParseInt(hex[0:2], 16, 0)
		gVal, _ := strconv.ParseInt(hex[2:4], 16, 0)
		bVal, _ := strconv.ParseInt(hex[4:6], 16, 0)
		r = float64(rVal) / 255
		g = float64(gVal) / 255
		b = float64(bVal) / 255
	case 8:
		// #RRGGBBAA
		rVal, _ := strconv.ParseInt(hex[0:2], 16, 0)
		gVal, _ := strconv.ParseInt(hex[2:4], 16, 0)
		bVal, _ := strconv.ParseInt(hex[4:6], 16, 0)
		aVal, _ := strconv.ParseInt(hex[6:8], 16, 0)
		r = float64(rVal) / 255
		g = float64(gVal) / 255
		b = float64(bVal) / 255
		a = float64(aVal) / 255
	default:
		return nil
	}

	return &Color{Red: r, Green: g, Blue: b, Alpha: a}
}

// parseRGBColor 解析 RGB 颜色
func parseRGBColor(rStr, gStr, bStr, aStr string) *Color {
	r, err1 := strconv.Atoi(rStr)
	g, err2 := strconv.Atoi(gStr)
	b, err3 := strconv.Atoi(bStr)
	a, err4 := strconv.ParseFloat(aStr, 64)

	if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
		return nil
	}

	return &Color{
		Red:   float64(r) / 255,
		Green: float64(g) / 255,
		Blue:  float64(b) / 255,
		Alpha: a,
	}
}

// parseHSLColor 解析 HSL 颜色
func parseHSLColor(hStr, sStr, lStr string) *Color {
	h, err1 := strconv.Atoi(hStr)
	s, err2 := strconv.Atoi(sStr)
	l, err3 := strconv.Atoi(lStr)

	if err1 != nil || err2 != nil || err3 != nil {
		return nil
	}

	// HSL to RGB 转换
	hNorm := float64(h) / 360
	sNorm := float64(s) / 100
	lNorm := float64(l) / 100

	var r, g, b float64

	if sNorm == 0 {
		r, g, b = lNorm, lNorm, lNorm
	} else {
		var q float64
		if lNorm < 0.5 {
			q = lNorm * (1 + sNorm)
		} else {
			q = lNorm + sNorm - lNorm*sNorm
		}
		p := 2*lNorm - q
		r = hueToRGB(p, q, hNorm+1.0/3)
		g = hueToRGB(p, q, hNorm)
		b = hueToRGB(p, q, hNorm-1.0/3)
	}

	return &Color{Red: r, Green: g, Blue: b, Alpha: 1}
}

// hueToRGB HSL 到 RGB 的辅助函数
func hueToRGB(p, q, t float64) float64 {
	if t < 0 {
		t += 1
	}
	if t > 1 {
		t -= 1
	}
	if t < 1.0/6 {
		return p + (q-p)*6*t
	}
	if t < 1.0/2 {
		return q
	}
	if t < 2.0/3 {
		return p + (q-p)*(2.0/3-t)*6
	}
	return p
}

// toHex 将整数转换为两位十六进制字符串
func toHex(n int) string {
	if n < 0 {
		n = 0
	}
	if n > 255 {
		n = 255
	}
	hex := strconv.FormatInt(int64(n), 16)
	if len(hex) == 1 {
		return "0" + hex
	}
	return hex
}

// formatFloat 格式化浮点数
func formatFloat(f float64) string {
	s := strconv.FormatFloat(f, 'f', 2, 64)
	// 去掉尾部的零
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	if s == "" {
		return "0"
	}
	return s
}
