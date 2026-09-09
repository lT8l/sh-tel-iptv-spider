package iptv

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
)

// MergeM3U8 preserves custom attributes by CommName and DisplayName, sorts
// entries known to the generated playlist, and appends truly old-only entries.
func MergeM3U8(existing, generated []byte) []byte {
	if len(bytes.TrimSpace(existing)) == 0 {
		return generated
	}

	oldList := parsePlaylist(existing)
	newList := parsePlaylist(generated)
	if len(newList.entries) == 0 {
		return existing
	}

	oldByEntry := make(map[playlistEntryKey][]int, len(oldList.entries))
	for i, entry := range oldList.entries {
		commName := entry.attr("tvg-id")
		if commName == "" {
			continue
		}
		key := playlistEntryKey{commName: commName, displayName: entry.displayName}
		oldByEntry[key] = append(oldByEntry[key], i)
	}

	used := make([]bool, len(oldList.entries))
	commOrder := make(map[string]int, len(newList.entries))
	ordered := make([]playlistOutputEntry, 0, len(newList.entries)+len(oldList.entries))
	for _, fresh := range newList.entries {
		commName := fresh.attr("tvg-id")
		order, ok := commOrder[commName]
		if !ok {
			order = len(commOrder)
			commOrder[commName] = order
		}

		key := playlistEntryKey{commName: commName, displayName: fresh.displayName}
		indices := oldByEntry[key]
		if len(indices) > 0 {
			idx := indices[0]
			oldByEntry[key] = indices[1:]
			used[idx] = true
			fresh = mergePlaylistEntry(oldList.entries[idx], fresh)
		}
		ordered = append(ordered, playlistOutputEntry{entry: fresh, modified: true, order: order})
	}

	unmatched := make([]playlistEntry, 0, len(oldList.entries))
	for i, entry := range oldList.entries {
		if used[i] {
			continue
		}
		commName := entry.attr("tvg-id")
		if commName != "" {
			if order, ok := commOrder[commName]; ok {
				ordered = append(ordered, playlistOutputEntry{entry: entry, order: order})
				continue
			}
		}
		unmatched = append(unmatched, entry)
	}

	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].order != ordered[j].order {
			return ordered[i].order < ordered[j].order
		}
		return ordered[i].entry.displayName < ordered[j].entry.displayName
	})

	header := mergePlaylistHeader(oldList.header, newList.header)
	return renderPlaylist(header, oldList.loose, ordered, unmatched)
}

type playlist struct {
	header  string
	loose   []string
	entries []playlistEntry
}

type playlistEntryKey struct {
	commName    string
	displayName string
}

type playlistOutputEntry struct {
	entry    playlistEntry
	modified bool
	order    int
}

type playlistEntry struct {
	duration    string
	attrs       []playlistAttr
	displayName string
	extras      []string
	url         string
	tail        []string
	raw         []string
}

type playlistAttr struct {
	key   string
	value string
	raw   string
}

func parsePlaylist(data []byte) playlist {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	var out playlist

	for i := 0; i < len(lines); {
		line := lines[i]
		if strings.HasPrefix(line, "#EXTM3U") && out.header == "" {
			out.header = line
			i++
			continue
		}
		if !strings.HasPrefix(line, "#EXTINF:") {
			out.loose = append(out.loose, line)
			i++
			continue
		}

		entry := parsePlaylistEntry(line)
		entry.raw = append(entry.raw, line)
		i++
		for i < len(lines) {
			next := lines[i]
			if strings.HasPrefix(next, "#EXTINF:") || strings.HasPrefix(next, "#EXTM3U") {
				break
			}
			entry.raw = append(entry.raw, next)
			if entry.url == "" && next != "" && !strings.HasPrefix(next, "#") {
				entry.url = next
			} else if entry.url == "" {
				entry.extras = append(entry.extras, next)
			} else {
				entry.tail = append(entry.tail, next)
			}
			i++
		}
		out.entries = append(out.entries, entry)
	}
	return out
}

func parsePlaylistEntry(line string) playlistEntry {
	left, name := splitExtInf(line)
	tokens := splitM3UTokens(left)
	entry := playlistEntry{displayName: strings.TrimSpace(name)}
	if len(tokens) > 0 {
		entry.duration = tokens[0]
	}
	for _, token := range tokens[1:] {
		entry.attrs = append(entry.attrs, parsePlaylistAttr(token))
	}
	if entry.displayName == "" {
		entry.displayName = entry.attr("tvg-name")
	}
	return entry
}

func splitExtInf(line string) (string, string) {
	body := strings.TrimPrefix(line, "#EXTINF:")
	quoted := false
	for i, r := range body {
		switch r {
		case '"':
			quoted = !quoted
		case ',':
			if !quoted {
				return strings.TrimSpace(body[:i]), body[i+1:]
			}
		}
	}
	return strings.TrimSpace(body), ""
}

func splitM3UTokens(s string) []string {
	var tokens []string
	start := -1
	quoted := false
	for i, r := range s {
		if r == '"' {
			quoted = !quoted
		}
		if !quoted && (r == ' ' || r == '\t') {
			if start >= 0 {
				tokens = append(tokens, s[start:i])
				start = -1
			}
			continue
		}
		if start < 0 {
			start = i
		}
	}
	if start >= 0 {
		tokens = append(tokens, s[start:])
	}
	return tokens
}

func parsePlaylistAttr(token string) playlistAttr {
	key, value, ok := strings.Cut(token, "=")
	if !ok {
		return playlistAttr{raw: token}
	}
	value = strings.TrimSpace(value)
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		value = strings.Trim(value, `"`)
	}
	return playlistAttr{key: key, value: value, raw: token}
}

func (e playlistEntry) attr(key string) string {
	for _, attr := range e.attrs {
		if strings.EqualFold(attr.key, key) {
			return attr.value
		}
	}
	return ""
}

func mergePlaylistEntry(old, fresh playlistEntry) playlistEntry {
	freshAttrs := make(map[string]playlistAttr, len(fresh.attrs))
	for _, attr := range fresh.attrs {
		if attr.key != "" {
			freshAttrs[strings.ToLower(attr.key)] = attr
		}
	}

	attrs := make([]playlistAttr, 0, len(old.attrs)+len(fresh.attrs))
	seen := make(map[string]bool, len(freshAttrs))
	for _, attr := range old.attrs {
		key := strings.ToLower(attr.key)
		if replacement, ok := freshAttrs[key]; ok {
			attrs = append(attrs, replacement)
			seen[key] = true
		} else if !isManagedPlaylistAttr(key) {
			attrs = append(attrs, attr)
		}
	}
	for _, attr := range fresh.attrs {
		key := strings.ToLower(attr.key)
		if key != "" && !seen[key] {
			attrs = append(attrs, attr)
			seen[key] = true
		}
	}

	old.attrs = attrs
	old.displayName = fresh.displayName
	old.url = fresh.url
	return old
}

func isManagedPlaylistAttr(key string) bool {
	switch key {
	case "tvg-id", "tvg-name", "group-title", "catchup", "catchup-days", "catchup-source":
		return true
	default:
		return false
	}
}

func mergePlaylistHeader(old, fresh string) string {
	if old == "" {
		return fresh
	}
	if fresh == "" {
		return old
	}

	oldAttrs := parseHeaderAttrs(old)
	freshAttrs := parseHeaderAttrs(fresh)
	freshByKey := make(map[string]playlistAttr, len(freshAttrs))
	for _, attr := range freshAttrs {
		if attr.key != "" {
			freshByKey[strings.ToLower(attr.key)] = attr
		}
	}
	seen := make(map[string]bool, len(freshByKey))
	merged := make([]playlistAttr, 0, len(oldAttrs)+len(freshAttrs))
	for _, attr := range oldAttrs {
		key := strings.ToLower(attr.key)
		if replacement, ok := freshByKey[key]; ok {
			merged = append(merged, replacement)
			seen[key] = true
		} else {
			merged = append(merged, attr)
		}
	}
	for _, attr := range freshAttrs {
		key := strings.ToLower(attr.key)
		if key != "" && !seen[key] {
			merged = append(merged, attr)
			seen[key] = true
		}
	}
	return renderHeader(merged)
}

func parseHeaderAttrs(line string) []playlistAttr {
	body := strings.TrimSpace(strings.TrimPrefix(line, "#EXTM3U"))
	if body == "" {
		return nil
	}
	tokens := splitM3UTokens(body)
	attrs := make([]playlistAttr, 0, len(tokens))
	for _, token := range tokens {
		attrs = append(attrs, parsePlaylistAttr(token))
	}
	return attrs
}

func renderHeader(attrs []playlistAttr) string {
	if len(attrs) == 0 {
		return "#EXTM3U"
	}
	var b strings.Builder
	b.WriteString("#EXTM3U")
	for _, attr := range attrs {
		b.WriteByte(' ')
		b.WriteString(renderPlaylistAttr(attr))
	}
	return b.String()
}

func renderPlaylist(header string, loose []string, ordered []playlistOutputEntry, unmatched []playlistEntry) []byte {
	if header == "" {
		header = "#EXTM3U"
	}
	var b strings.Builder
	b.WriteString(header)
	b.WriteByte('\n')
	for _, line := range loose {
		if line == "" {
			continue
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	for _, item := range ordered {
		renderPlaylistEntry(&b, item.entry, item.modified)
	}
	for _, entry := range unmatched {
		renderPlaylistEntry(&b, entry, false)
	}
	return []byte(b.String())
}

func renderPlaylistEntry(b *strings.Builder, entry playlistEntry, modified bool) {
	if !modified && len(entry.raw) > 0 {
		for _, line := range entry.raw {
			b.WriteString(line)
			b.WriteByte('\n')
		}
		return
	}

	duration := entry.duration
	if duration == "" {
		duration = "-1"
	}
	fmt.Fprintf(b, "#EXTINF:%s", duration)
	for _, attr := range entry.attrs {
		b.WriteByte(' ')
		b.WriteString(renderPlaylistAttr(attr))
	}
	fmt.Fprintf(b, ",%s\n", entry.displayName)
	for _, line := range entry.extras {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	if entry.url != "" {
		b.WriteString(entry.url)
		b.WriteByte('\n')
	}
	for _, line := range entry.tail {
		b.WriteString(line)
		b.WriteByte('\n')
	}
}

func renderPlaylistAttr(attr playlistAttr) string {
	if attr.raw != "" {
		return attr.raw
	}
	if attr.key == "" {
		return ""
	}
	return fmt.Sprintf("%s=%q", attr.key, attr.value)
}
