package config

import (
	"fmt"
	"strings"
)

// JSONErrorInfo holds information about a JSON parsing error
type JSONErrorInfo struct {
	Line       int
	Column     int
	LineContent string
	Message    string
}

// CalculateLineColumn calculates line and column from byte offset
func CalculateLineColumn(data []byte, offset int64) (line, column int, lineContent string) {
	if offset < 0 || int(offset) > len(data) {
		return 0, 0, ""
	}

	// Count newlines before offset
	line = 1
	lineStart := 0
	for i := int64(0); i < offset && i < int64(len(data)); i++ {
		if data[i] == '\n' {
			line++
			lineStart = int(i) + 1
		}
	}

	column = int(offset) - lineStart + 1

	// Extract the line content
	lineEnd := int(offset)
	for lineEnd < len(data) && data[lineEnd] != '\n' {
		lineEnd++
	}

	if lineStart < len(data) {
		end := lineEnd
		if end > lineStart+100 {
			end = lineStart + 100
		}
		lineContent = string(data[lineStart:end])
	}

	return line, column, lineContent
}

// FormatJSONParseError formats a JSON parse error with line numbers and context
func FormatJSONParseError(data []byte, err error) string {
	if err == nil {
		return ""
	}

	errMsg := err.Error()

	// Try to extract character and position from error message
	// Go json error format: "invalid character 'x' looking for ..."
	var offset int64 = -1

	// Try multiple strategies to extract offset
	// Strategy 1: Look for explicit offset mention
	if idx := strings.Index(errMsg, "offset "); idx != -1 {
		fmt.Sscanf(errMsg[idx:], "offset %d", &offset)
	}

	// Strategy 2: Parse the error and find the position by trying to locate
	// the problematic character in the data
	if offset < 0 {
		// Direct approach: look for common problematic characters
		// For single quote errors, find first single quote in data
		if strings.Contains(errMsg, "'\\''") || strings.Contains(errMsg, "invalid character") {
			for i := 0; i < len(data); i++ {
				if data[i] == '\'' {
					offset = int64(i)
					break
				}
			}
		}
	}

	// Strategy 2.5: Handle "looking for beginning" errors
	if offset < 0 && strings.Contains(errMsg, "looking for beginning") {
		// Find the character that's causing the issue
		// Error format: invalid character '}' looking for beginning of object key string
		parts := strings.Split(errMsg, "'")
		if len(parts) >= 2 {
			ch := parts[1] // The character between quotes
			// Find first occurrence after a value (trailing comma scenario)
			for i := 1; i < len(data); i++ {
				if string(data[i]) == ch {
					// Check if previous non-whitespace is a comma or value end
					for j := i - 1; j >= 0; j-- {
						if data[j] != ' ' && data[j] != '\t' && data[j] != '\n' && data[j] != '\r' {
							if data[j] == ',' || data[j] == '"' || data[j] == '}' || data[j] == ']' {
								offset = int64(i)
								break
							}
							break
						}
					}
					if offset > 0 {
						break
					}
				}
			}
		}
	}

	// Strategy 3: Find common error patterns
	if offset < 0 {
		// Look for patterns like "after object key:value pair"
		// This usually means a missing comma after } or before "
		if strings.Contains(errMsg, "after object") || strings.Contains(errMsg, "after array") {
			// Find the position where a comma should be (after } or ] before a new key)
			for i := 1; i < len(data)-1; i++ {
				// Look for pattern: }\n" or }\n  " (missing comma before a key)
				if (data[i] == '}' || data[i] == ']') && i+1 < len(data) {
					// Check if next non-whitespace is a quote (new key)
					for j := i + 1; j < len(data); j++ {
						if data[j] == '"' {
							// Found the problematic quote
							offset = int64(j)
							break
						} else if data[j] == ',' || data[j] == '}' || data[j] == ']' {
							// Found a comma or closing bracket, not our issue
							break
						}
					}
					if offset > 0 {
						break
					}
				}
			}
		}
	}

	if offset >= 0 && offset < int64(len(data)) {
		line, column, lineContent := CalculateLineColumn(data, offset)

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("JSON 语法错误在第 %d 行第 %d 列\n", line, column))
		sb.WriteString(fmt.Sprintf("错误详情: %s\n", errMsg))

		if lineContent != "" {
			sb.WriteString(fmt.Sprintf("\n错误位置 (第 %d 行):\n", line))
			sb.WriteString(fmt.Sprintf("  %s\n", strings.TrimSpace(lineContent)))

			// Add pointer to the error position
			if column > 0 {
				pointer := strings.Repeat(" ", column+1) + "^"
				sb.WriteString(fmt.Sprintf("  %s\n", pointer))
			}
		}

		sb.WriteString("\n常见错误原因:")
		sb.WriteString("\n  1. 缺少逗号 (,) - 在对象或数组的元素之间需要逗号分隔")
		sb.WriteString("\n  2. 多余逗号 - 最后一个元素后不应有逗号")
		sb.WriteString("\n  3. 引号问题 - 键和字符串值必须使用双引号 \"\"，不能使用单引号")
		sb.WriteString("\n  4. 括号不匹配 - 检查 {} 和 [] 是否正确配对")
		sb.WriteString("\n  5. 使用了注释 - JSON 标准不支持注释")
		sb.WriteString("\n  6. 特殊字符未转义 - 如字符串中的引号需要转义")

		return sb.String()
	}

	// Fallback: return generic error message
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("JSON 解析失败: %s\n\n", errMsg))
	sb.WriteString("可能的原因:")
	sb.WriteString("\n  - 文件格式不正确，请使用 JSON 验证工具检查")
	sb.WriteString("\n  - 推荐在线工具: https://jsonlint.com/")
	sb.WriteString("\n  - 或使用命令: python -m json.tool config.json")
	sb.WriteString("\n  - 或使用命令: cat config.json | jq .")

	return sb.String()
}
