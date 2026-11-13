package utils

import "strings"

// asciiWordByte reports whether c belongs to Go regexp's ASCII \w class. It is
// also the character class Java local names are made of, so a literal word
// match over these bytes reproduces `\b`+QuoteMeta(name)+`\b` exactly.
func asciiWordByte(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// wordBoundaryAt reports whether pos is a Go regexp word boundary: the start or
// end of text next to a word byte, or a transition between word and non-word.
func wordBoundaryAt(text string, pos int) bool {
	if pos <= 0 {
		return pos == 0 && len(text) > 0 && asciiWordByte(text[0])
	}
	if pos >= len(text) {
		return pos == len(text) && asciiWordByte(text[len(text)-1])
	}
	return asciiWordByte(text[pos-1]) != asciiWordByte(text[pos])
}

// nextWordSpan returns the earliest whole-word occurrence of word at or after
// offset, or (-1, -1) when there is none.
func nextWordSpan(text, word string, offset int) (int, int) {
	for offset+len(word) <= len(text) {
		rel := strings.Index(text[offset:], word)
		if rel < 0 {
			return -1, -1
		}
		start := offset + rel
		end := start + len(word)
		if wordBoundaryAt(text, start) && wordBoundaryAt(text, end) {
			return start, end
		}
		offset = start + 1
	}
	return -1, -1
}

// wordBoundaries appends every zero-width word-boundary position. It matches
// FindAllStringIndex for the degenerate pattern `\b\b`, which is what an empty
// name compiles to.
func wordBoundaries(text string, dst [][2]int) [][2]int {
	if len(text) == 0 {
		return dst
	}
	for pos := 0; pos <= len(text); pos++ {
		if wordBoundaryAt(text, pos) {
			dst = append(dst, [2]int{pos, pos})
		}
	}
	return dst
}

// WordMatches reports whether text contains word as a whole token, matching
// `\b`+QuoteMeta(word)+`\b`.
func WordMatches(text, word string) bool {
	if word == "" {
		return len(wordBoundaries(text, nil)) > 0
	}
	start, _ := nextWordSpan(text, word, 0)
	return start >= 0
}

// WordCount counts non-overlapping whole-token occurrences of word in text,
// matching regexp.FindAllString(..., -1) on `\b`+QuoteMeta(word)+`\b`.
func WordCount(text, word string) int {
	if word == "" {
		return len(wordBoundaries(text, nil))
	}
	count := 0
	for offset := 0; offset+len(word) <= len(text); {
		start, end := nextWordSpan(text, word, offset)
		if start < 0 {
			break
		}
		count++
		offset = end
	}
	return count
}

// WordSpans appends the byte ranges of non-overlapping whole-token occurrences
// of word to dst, matching FindAllStringIndex on `\b`+QuoteMeta(word)+`\b`.
// Passing a nil dst still saves the caller the regexp compile on the common
// no-match path.
func WordSpans(text, word string, dst [][2]int) [][2]int {
	if word == "" {
		return wordBoundaries(text, dst)
	}
	for offset := 0; offset+len(word) <= len(text); {
		start, end := nextWordSpan(text, word, offset)
		if start < 0 {
			break
		}
		dst = append(dst, [2]int{start, end})
		offset = end
	}
	return dst
}
