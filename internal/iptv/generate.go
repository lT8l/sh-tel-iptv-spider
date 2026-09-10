package iptv

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type M3UOptions struct {
	StreamMode      string
	EnableFCC       bool
	CatchupDays     int
	CatchupTemplate string
	IncludeShopping bool
}

type XMLTVOptions struct {
	Generator   string
	Source      string
	HistoryDays int
	Location    *time.Location
}

type xmlTV struct {
	XMLName   xml.Name       `xml:"tv"`
	Generator string         `xml:"generator-info-name,attr"`
	Source    string         `xml:"source-info-name,attr"`
	Channels  []xmlTVChannel `xml:"channel"`
	Programs  []xmlTVProgram `xml:"programme"`
}

type xmlTVChannel struct {
	ID          string           `xml:"id,attr"`
	DisplayName []xmlTVTextField `xml:"display-name"`
}

type xmlTVProgram struct {
	Start   string           `xml:"start,attr"`
	Stop    string           `xml:"stop,attr"`
	Channel string           `xml:"channel,attr"`
	Title   []xmlTVTextField `xml:"title"`
	Desc    []xmlTVTextField `xml:"desc"`
}

type xmlTVTextField struct {
	Lang  string `xml:"lang,attr"`
	Value string `xml:",chardata"`
}

func GenerateM3U8(infos []ChannelInfo, channels []Channel, opts M3UOptions) []byte {
	byUserID := make(map[string]Channel, len(channels))
	for _, ch := range channels {
		byUserID[ch.UserChannelID] = ch
	}

	ordered := append([]ChannelInfo(nil), infos...)
	sortM3UChannelInfos(ordered)

	var b strings.Builder
	b.WriteString("#EXTM3U\n")
	for _, info := range ordered {
		group := autoGroup(info)
		if !opts.IncludeShopping && group == "购物" {
			continue
		}
		stream, ok := byUserID[info.MixNo]
		if !ok {
			continue
		}
		streamURL := transformStreamURL(stream, opts)
		if streamURL == "" {
			continue
		}

		fmt.Fprintf(&b, "#EXTINF:-1 tvg-id=%q tvg-name=%q", info.CommName, info.DisplayName)
		if group != "" {
			fmt.Fprintf(&b, " group-title=%q", group)
		}
		if catchupURL := transformCatchupURL(stream.TimeShiftURL, opts); catchupURL != "" {
			fmt.Fprintf(&b, " catchup=%q catchup-days=%q catchup-source=%q",
				"default", strconv.Itoa(opts.CatchupDays), catchupURL)
		}
		fmt.Fprintf(&b, ",%s\n%s\n", info.DisplayName, streamURL)
	}
	return []byte(b.String())
}

func sortM3UChannelInfos(in []ChannelInfo) {
	sort.SliceStable(in, func(i, j int) bool {
		a, b := in[i], in[j]
		if lessMixNo(a.MixNo, b.MixNo) {
			return true
		}
		if lessMixNo(b.MixNo, a.MixNo) {
			return false
		}
		return a.DisplayName < b.DisplayName
	})
}

func transformStreamURL(stream Channel, opts M3UOptions) string {
	raw := strings.TrimSpace(stream.ChannelURL)
	if raw == "" || strings.EqualFold(raw, "null") {
		return ""
	}
	if opts.StreamMode == "raw" {
		return raw
	}
	if opts.StreamMode != "rtp" && opts.StreamMode != "udp" {
		return ""
	}

	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	liveURL := opts.StreamMode + "://" + u.Host
	if opts.StreamMode == "rtp" && opts.EnableFCC {
		liveURL = appendFCC(liveURL, stream)
	}
	return liveURL
}

func appendFCC(liveURL string, stream Channel) string {
	ip := strings.TrimSpace(stream.ChannelFCCIP)
	port := strings.TrimSpace(stream.ChannelFCCPort)
	if ip == "" || port == "" || strings.EqualFold(ip, "null") || strings.EqualFold(port, "null") {
		return liveURL
	}
	return appendRawQuery(liveURL, "fcc="+net.JoinHostPort(ip, port))
}

func transformCatchupURL(raw string, opts M3UOptions) string {
	raw = strings.TrimSpace(raw)
	seek := strings.TrimSpace(opts.CatchupTemplate)
	if opts.CatchupDays <= 0 || raw == "" || strings.EqualFold(raw, "null") || seek == "" {
		return ""
	}

	u, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(u.Scheme, "rtsp") || u.Hostname() == "" {
		return ""
	}
	u.RawQuery = appendQueryValue(u.RawQuery, seek)
	return u.String()
}

func appendRawQuery(rawURL, value string) string {
	if strings.Contains(rawURL, "?") {
		return rawURL + "&" + value
	}
	return rawURL + "?" + value
}

func appendQueryValue(query, value string) string {
	if query == "" {
		return value
	}
	return query + "&" + value
}

func GenerateXMLTV(infos []ChannelInfo, epg map[string][]EPGDetails, opts XMLTVOptions) ([]byte, error) {
	if opts.Location == nil {
		opts.Location = time.Local
	}
	if opts.HistoryDays < 1 {
		opts.HistoryDays = 1
	}
	now := time.Now().In(opts.Location)
	cutoff := now.AddDate(0, 0, -opts.HistoryDays).UnixMilli()
	tv := xmlTV{
		Generator: fmt.Sprintf("%s %s", opts.Generator, now.Format("2006-01-02 15:04:05")),
		Source:    opts.Source,
	}
	for _, info := range DedupeEPGChannelInfos(infos) {
		tv.Channels = append(tv.Channels, xmlTVChannel{
			ID:          info.CommName,
			DisplayName: []xmlTVTextField{{Lang: "zh", Value: info.DisplayName}},
		})
		programs := append([]EPGDetails(nil), epg[info.MixNo]...)
		sort.Slice(programs, func(i, j int) bool { return programs[i].StartTime < programs[j].StartTime })
		for _, p := range programs {
			if p.EndTime <= cutoff {
				continue
			}
			tv.Programs = append(tv.Programs, xmlTVProgram{
				Start:   time.UnixMilli(p.StartTime).In(opts.Location).Format("20060102150405 -0700"),
				Stop:    time.UnixMilli(p.EndTime).In(opts.Location).Format("20060102150405 -0700"),
				Channel: info.CommName,
				Title:   []xmlTVTextField{{Lang: "zh", Value: p.Name}},
				Desc:    []xmlTVTextField{{Lang: "zh"}},
			})
		}
	}
	body, err := xml.MarshalIndent(tv, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal XMLTV: %w", err)
	}
	var out bytes.Buffer
	out.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	out.WriteByte('\n')
	out.Write(body)
	out.WriteByte('\n')
	return out.Bytes(), nil
}

func WriteStaticFiles(m3uPath string, m3u []byte, xmlPath string, xmlData []byte) (string, string, error) {
	if len(parsePlaylist(m3u).entries) == 0 {
		return "", "", fmt.Errorf("generated M3U8 contains no playable channels")
	}
	if err := os.MkdirAll(filepath.Dir(m3uPath), 0o755); err != nil {
		return "", "", fmt.Errorf("create output directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(xmlPath), 0o755); err != nil {
		return "", "", fmt.Errorf("create output directory: %w", err)
	}

	existing, err := os.ReadFile(m3uPath)
	if err == nil {
		m3u = MergeM3U8(existing, m3u)
	} else if !os.IsNotExist(err) {
		return "", "", fmt.Errorf("read existing %s: %w", m3uPath, err)
	}
	if err := writeAtomic(m3uPath, m3u); err != nil {
		return "", "", err
	}
	if err := writeAtomic(xmlPath, xmlData); err != nil {
		return "", "", err
	}
	return m3uPath, xmlPath, nil
}

func writeAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".iptv-static-*")
	if err != nil {
		return fmt.Errorf("create temp file for %s: %w", path, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}
