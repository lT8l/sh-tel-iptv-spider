package iptv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGenerateM3U8SortsByMixNoAndSharesCommName(t *testing.T) {
	infos := []ChannelInfo{
		{MixNo: "10", CommName: "CCTV-1", DisplayName: "CCTV-1 HD 高清"},
		{MixNo: "3", CommName: "东方卫视", DisplayName: "东方卫视"},
		{MixNo: "10", CommName: "CCTV-1", DisplayName: "CCTV-1 4K 超清"},
	}
	channels := []Channel{
		{UserChannelID: "3", ChannelURL: "igmp://239.0.0.3:5140"},
		{UserChannelID: "10", ChannelURL: "igmp://239.0.0.10:5140"},
	}
	got := string(GenerateM3U8(infos, channels, M3UOptions{StreamMode: "rtp"}))
	if !strings.HasPrefix(got, "#EXTM3U\n") || strings.Contains(got, "url-tvg") || strings.Contains(got, "x-tvg-url") {
		t.Fatalf("unexpected M3U header:\n%s", got)
	}
	if strings.Count(got, `tvg-id="CCTV-1"`) != 2 {
		t.Fatalf("quality variants should share tvg-id:\n%s", got)
	}
	if strings.Index(got, `tvg-name="东方卫视"`) > strings.Index(got, `tvg-name="CCTV-1 HD 高清"`) {
		t.Fatalf("channels not sorted by MixNo:\n%s", got)
	}
}

func TestGenerateM3U8SortsSameMixNoByDisplayName(t *testing.T) {
	infos := []ChannelInfo{
		{MixNo: "10", CommName: "TEN", DisplayName: "Z频道"},
		{MixNo: "2", CommName: "FIRST", DisplayName: "先频道"},
		{MixNo: "10", CommName: "TEN", DisplayName: "A频道"},
	}
	channels := []Channel{
		{UserChannelID: "2", ChannelURL: "igmp://239.0.0.2:5140"},
		{UserChannelID: "10", ChannelURL: "igmp://239.0.0.10:5140"},
	}

	got := string(GenerateM3U8(infos, channels, M3UOptions{StreamMode: "rtp"}))
	first := strings.Index(got, `tvg-name="先频道"`)
	a := strings.Index(got, `tvg-name="A频道"`)
	z := strings.Index(got, `tvg-name="Z频道"`)
	if first < 0 || a < 0 || z < 0 || !(first < a && a < z) {
		t.Fatalf("M3U8 should sort by MixNo then DisplayName:\n%s", got)
	}
}

func TestGenerateXMLTVDedupesSharedMixNoAndUsesDisplayName(t *testing.T) {
	loc := time.FixedZone("CST", 8*60*60)
	now := time.Now()
	infos := []ChannelInfo{
		{MixNo: "10", CommName: "CCTV-1", DisplayName: "CCTV-1 HD 高清"},
		{MixNo: "10", CommName: "CCTV-1", DisplayName: "CCTV-1 4K 超清"},
	}
	epg := map[string][]EPGDetails{
		"10": {{Name: "新闻", StartTime: now.Add(-time.Hour).UnixMilli(), EndTime: now.Add(time.Hour).UnixMilli()}},
	}
	got, err := GenerateXMLTV(infos, epg, XMLTVOptions{Generator: "test", Source: "test", HistoryDays: 1, Location: loc})
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	if strings.Count(text, `<channel id="CCTV-1">`) != 1 {
		t.Fatalf("shared XMLTV channel not deduped:\n%s", text)
	}
	if !strings.Contains(text, `<display-name lang="zh">CCTV-1 HD 高清</display-name>`) {
		t.Fatalf("EPG DisplayName not used:\n%s", text)
	}
	if strings.Count(text, `channel="CCTV-1"`) != 1 || !strings.Contains(text, ">新闻<") {
		t.Fatalf("shared programme duplicated or missing:\n%s", text)
	}
}

func TestFCCOnlyAppliesToRTP(t *testing.T) {
	stream := Channel{
		ChannelURL:     "igmp://239.0.0.1:5140",
		ChannelFCCIP:   "10.0.0.1",
		ChannelFCCPort: "15970",
	}
	rtp := transformStreamURL(stream, M3UOptions{StreamMode: "rtp", EnableFCC: true})
	if rtp != "rtp://239.0.0.1:5140?fcc=10.0.0.1:15970" {
		t.Fatalf("rtp=%q", rtp)
	}
	udp := transformStreamURL(stream, M3UOptions{StreamMode: "udp", EnableFCC: true})
	if udp != "udp://239.0.0.1:5140" {
		t.Fatalf("udp=%q", udp)
	}
}

func TestTransformStreamURLDoesNotFallbackToRawForInvalidURL(t *testing.T) {
	stream := Channel{ChannelURL: "not-a-stream-url"}
	if got := transformStreamURL(stream, M3UOptions{StreamMode: "rtp"}); got != "" {
		t.Fatalf("rtp invalid URL=%q, want empty", got)
	}
	if got := transformStreamURL(stream, M3UOptions{StreamMode: "raw"}); got != stream.ChannelURL {
		t.Fatalf("raw URL=%q, want %q", got, stream.ChannelURL)
	}
}

func TestWriteStaticFilesRejectsEmptyGeneratedM3U8(t *testing.T) {
	dir := t.TempDir()
	m3uPath := filepath.Join(dir, "channels.m3u8")
	xmlPath := filepath.Join(dir, "channels.xml")
	if _, _, err := WriteStaticFiles(m3uPath, []byte("#EXTM3U\n"), xmlPath, []byte("<tv/>\n")); err == nil {
		t.Fatal("expected empty generated M3U8 to fail")
	}
	if _, err := os.Stat(m3uPath); !os.IsNotExist(err) {
		t.Fatalf("M3U8 should not be written on validation failure: %v", err)
	}
	if _, err := os.Stat(xmlPath); !os.IsNotExist(err) {
		t.Fatalf("XMLTV should not be written on validation failure: %v", err)
	}
}
