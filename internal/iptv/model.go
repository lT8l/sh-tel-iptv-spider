package iptv

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type Channel struct {
	UserChannelID  string `key:"UserChannelID"`
	ChannelURL     string `key:"ChannelURL"`
	TimeShiftURL   string `key:"TimeShiftURL"`
	ChannelFCCPort string `key:"ChannelFCCPort"`
	ChannelFCCIP   string `key:"ChannelFCCIP"`
}

type ChannelInfo struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	ChID        string `json:"ID"`
	MixNo       string `json:"mixNo"`
	IsCharge    string `json:"isCharge"`
	IsHD        bool   `json:"-"`
	Is4K        bool   `json:"-"`
	CommName    string `json:"-"`
	DisplayName string `json:"-"`
}

type EPGDetails struct {
	Name      string `json:"name"`
	StartTime int64  `json:"startTime"`
	EndTime   int64  `json:"endTime"`
}

type jsonResponse[T any] struct {
	ErrCode string `json:"errCode"`
	ErrMsg  string `json:"errMsg"`
	Data    []T    `json:"data"`
}

func (r jsonResponse[T]) err() error {
	code := strings.TrimSpace(r.ErrCode)
	if code == "" || strings.Trim(code, "0") == "" {
		return nil
	}
	message := strings.TrimSpace(r.ErrMsg)
	if message == "" {
		return fmt.Errorf("IPTV API error %s", code)
	}
	return fmt.Errorf("IPTV API error %s: %s", code, message)
}

func normalizeChannelInfo(ch ChannelInfo) ChannelInfo {
	name := strings.ToUpper(strings.TrimSpace(ch.Name))
	base := name

	switch {
	case strings.HasSuffix(name, "4K"):
		ch.Is4K = true
		ch.IsHD = true
		base = trimQualitySuffix(name, "4K")
		ch.DisplayName = base + " 4K 超清"
	case strings.HasSuffix(name, "HD"):
		ch.IsHD = true
		base = trimQualitySuffix(name, "HD")
		ch.DisplayName = base + " HD 高清"
	case strings.HasSuffix(name, "(高清)"):
		ch.IsHD = true
		base = strings.TrimSpace(strings.TrimSuffix(name, "(高清)"))
		ch.DisplayName = base + " HD 高清"
	case strings.HasSuffix(name, "（高清）"):
		ch.IsHD = true
		base = strings.TrimSpace(strings.TrimSuffix(name, "（高清）"))
		ch.DisplayName = base + " HD 高清"
	case strings.HasSuffix(name, "高清"):
		ch.IsHD = true
		base = strings.TrimSpace(strings.TrimSuffix(name, "高清"))
		ch.DisplayName = base + " HD 高清"
	default:
		ch.DisplayName = base
	}

	ch.CommName = base
	return ch
}

func trimQualitySuffix(name, suffix string) string {
	base := strings.TrimSpace(strings.TrimSuffix(name, suffix))
	return strings.TrimSpace(strings.TrimRight(base, "-_"))
}

// DedupeChannelInfos keeps one channel per CommName and quality tier. HD and 4K
// may coexist; either enhanced tier suppresses the SD variant. Variants of the
// same CommName share the same MixNo, so MixNo is not used as a preference.
func DedupeChannelInfos(in []ChannelInfo) []ChannelInfo {
	type variants struct {
		sd, hd, uhd         ChannelInfo
		hasSD, hasHD, has4K bool
	}

	groups := make(map[string]variants, len(in))
	for _, ch := range in {
		v := groups[ch.CommName]
		switch {
		case ch.Is4K:
			if !v.has4K {
				v.uhd = ch
				v.has4K = true
			}
		case ch.IsHD:
			if !v.hasHD {
				v.hd = ch
				v.hasHD = true
			}
		default:
			if !v.hasSD {
				v.sd = ch
				v.hasSD = true
			}
		}
		groups[ch.CommName] = v
	}

	out := make([]ChannelInfo, 0, len(groups)*2)
	for _, v := range groups {
		if v.has4K {
			out = append(out, v.uhd)
		}
		if v.hasHD {
			out = append(out, v.hd)
		}
		if !v.has4K && !v.hasHD && v.hasSD {
			out = append(out, v.sd)
		}
	}
	sortChannelInfos(out)
	return out
}

// DedupeEPGChannelInfos keeps one request entry per MixNo. Quality variants
// of the same logical channel share MixNo and therefore the same EPG.
func DedupeEPGChannelInfos(in []ChannelInfo) []ChannelInfo {
	seen := make(map[string]struct{}, len(in))
	out := make([]ChannelInfo, 0, len(in))
	for _, ch := range in {
		if ch.MixNo == "" {
			continue
		}
		if _, ok := seen[ch.MixNo]; ok {
			continue
		}
		seen[ch.MixNo] = struct{}{}
		out = append(out, ch)
	}
	sortChannelInfos(out)
	return out
}

func sortChannelInfos(in []ChannelInfo) {
	sort.SliceStable(in, func(i, j int) bool {
		a, b := in[i], in[j]
		if lessMixNo(a.MixNo, b.MixNo) {
			return true
		}
		if lessMixNo(b.MixNo, a.MixNo) {
			return false
		}
		if a.Is4K != b.Is4K {
			return a.Is4K
		}
		if a.IsHD != b.IsHD {
			return a.IsHD
		}
		if a.DisplayName != b.DisplayName {
			return a.DisplayName < b.DisplayName
		}
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		return a.ChID < b.ChID
	})
}

func lessMixNo(a, b string) bool {
	ai, aErr := strconv.Atoi(a)
	bi, bErr := strconv.Atoi(b)
	switch {
	case aErr == nil && bErr == nil:
		return ai < bi
	case aErr == nil:
		return true
	case bErr == nil:
		return false
	default:
		return a < b
	}
}

func autoGroup(ch ChannelInfo) string {
	switch {
	case strings.Contains(ch.CommName, "CCTV"):
		return "央视"
	case strings.Contains(ch.Name, "卫视"):
		return "卫视"
	case strings.Contains(ch.Name, "购物"):
		return "购物"
	case strings.Contains(ch.Name, "年级"):
		return "空中课堂"
	case strings.Contains(ch.Name, "百事通"):
		return "百事通"
	default:
		return ""
	}
}
