package textbody

import "strings"

// Normalize converts accidental escaped newline sequences in Markdown-capable
// bodies while preserving common quoted/code contexts where literal escapes are
// likely intentional.
func Normalize(body string) string {
	if !strings.Contains(body, `\n`) && !strings.Contains(body, `\r`) {
		return body
	}

	var out strings.Builder
	out.Grow(len(body))
	inSingleQuote := false
	inDoubleQuote := false
	inBacktick := false
	inFence := false
	escapedInQuote := false

	for i := 0; i < len(body); i++ {
		ch := body[i]
		inProtectedContext := inSingleQuote || inDoubleQuote || inBacktick || inFence

		if ch == '`' && !inSingleQuote && !inDoubleQuote {
			runLength := 1
			for i+runLength < len(body) && body[i+runLength] == '`' {
				runLength++
			}
			if runLength >= 3 {
				inFence = !inFence
				out.WriteString(body[i : i+runLength])
				i += runLength - 1
				escapedInQuote = false
				continue
			}
		}

		if ch == '\\' && !inProtectedContext && (i == 0 || body[i-1] != '\\') && i+1 < len(body) {
			switch body[i+1] {
			case 'n':
				out.WriteByte('\n')
				i++
				continue
			case 'r':
				out.WriteByte('\n')
				i++
				if i+1 < len(body) && body[i+1] == '\\' && i+2 < len(body) && body[i+2] == 'n' {
					i += 2
				}
				continue
			}
		}

		if ch == '`' && !inSingleQuote && !inDoubleQuote && !inFence {
			inBacktick = !inBacktick
		} else if ch == '\'' && !inDoubleQuote && !inBacktick && !inFence && !escapedInQuote {
			if inSingleQuote && isSingleQuoteClosingBoundary(body, i) {
				inSingleQuote = false
			} else if !inSingleQuote && isSingleQuoteOpeningBoundary(body, i) {
				inSingleQuote = true
			}
		} else if ch == '"' && !inSingleQuote && !inBacktick && !inFence && !escapedInQuote {
			inDoubleQuote = !inDoubleQuote
		}

		out.WriteByte(ch)

		if (inSingleQuote || inDoubleQuote) && ch == '\\' && !escapedInQuote {
			escapedInQuote = true
		} else {
			escapedInQuote = false
		}
	}

	return out.String()
}

func isSingleQuoteOpeningBoundary(s string, i int) bool {
	if i+1 >= len(s) || isASCIISpace(s[i+1]) {
		return false
	}
	return i == 0 || isASCIISpace(s[i-1]) || strings.ContainsRune("([{=:", rune(s[i-1]))
}

func isSingleQuoteClosingBoundary(s string, i int) bool {
	return i+1 == len(s) || isASCIISpace(s[i+1]) || strings.ContainsRune(")]},.;:", rune(s[i+1])) || isEscapedNewlineAt(s, i+1)
}

func isEscapedNewlineAt(s string, i int) bool {
	return i+1 < len(s) && s[i] == '\\' && (s[i+1] == 'n' || s[i+1] == 'r')
}

func isASCIISpace(ch byte) bool {
	switch ch {
	case ' ', '\t', '\n', '\r', '\f', '\v':
		return true
	default:
		return false
	}
}
