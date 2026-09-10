package iptv

import (
	"strings"
	"testing"
)

func TestMergeM3U8DropsStaleManagedAttrsAndKeepsCustomAttrs(t *testing.T) {
	existing := []byte(`#EXTM3U url-tvg="manual.xml"
#EXTINF:-1 tvg-id="OLD" tvg-name="CCTV-1 HD 高清" group-title="旧组" catchup="default" catchup-days="7" catchup-source="rtsp://old" tvg-logo="logo.png" custom="keep",CCTV-1 HD 高清
rtp://239.0.0.1:5140
`)
	generated := []byte(`#EXTM3U
#EXTINF:-1 tvg-id="OLD" tvg-name="CCTV-1 HD 高清",CCTV-1 HD 高清
rtp://239.0.0.2:5140
`)

	got := string(MergeM3U8(existing, generated))
	for _, stale := range []string{`group-title="旧组"`, `catchup="default"`, `catchup-days="7"`, `catchup-source="rtsp://old"`} {
		if strings.Contains(got, stale) {
			t.Fatalf("stale managed attribute %s was preserved:\n%s", stale, got)
		}
	}
	for _, kept := range []string{`url-tvg="manual.xml"`, `tvg-logo="logo.png"`, `custom="keep"`, `tvg-id="OLD"`, `rtp://239.0.0.2:5140`} {
		if !strings.Contains(got, kept) {
			t.Fatalf("expected %s to be preserved or updated:\n%s", kept, got)
		}
	}
}

func TestMergeM3U8ReplacesManagedAttrsWhenGenerated(t *testing.T) {
	existing := []byte(`#EXTM3U
#EXTINF:-1 tvg-id="CCTV-1" tvg-name="CCTV-1 HD 高清" group-title="旧组" catchup-days="3" tvg-logo="logo.png",CCTV-1 HD 高清
rtp://239.0.0.1:5140
`)
	generated := []byte(`#EXTM3U
#EXTINF:-1 tvg-id="CCTV-1" tvg-name="CCTV-1 HD 高清" group-title="央视" catchup="default" catchup-days="7" catchup-source="rtsp://new",CCTV-1 HD 高清
rtp://239.0.0.2:5140
`)

	got := string(MergeM3U8(existing, generated))
	for _, want := range []string{`group-title="央视"`, `catchup="default"`, `catchup-days="7"`, `catchup-source="rtsp://new"`, `tvg-logo="logo.png"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %s:\n%s", want, got)
		}
	}
	if strings.Contains(got, `group-title="旧组"`) || strings.Contains(got, `catchup-days="3"`) {
		t.Fatalf("old managed attributes were not replaced:\n%s", got)
	}
}

func TestMergeM3U8KeepsGeneratedOrderAndAppendsOldOnly(t *testing.T) {
	existing := []byte(`#EXTM3U
#EXTINF:-1 tvg-id="B" tvg-name="B频道" custom="keep-b",B频道
rtp://239.0.0.20:5140
#EXTINF:-1 tvg-id="STALE" tvg-name="旧频道",旧频道
rtp://239.0.0.99:5140
#EXTINF:-1 tvg-id="A" tvg-name="A频道" custom="keep-a",A频道
rtp://239.0.0.10:5140
`)
	generated := []byte(`#EXTM3U
#EXTINF:-1 tvg-id="A" tvg-name="A频道",A频道
rtp://239.0.0.1:5140
#EXTINF:-1 tvg-id="B" tvg-name="B频道",B频道
rtp://239.0.0.2:5140
`)

	got := string(MergeM3U8(existing, generated))
	a := strings.Index(got, `tvg-id="A"`)
	b := strings.Index(got, `tvg-id="B"`)
	stale := strings.Index(got, `tvg-id="STALE"`)
	if a < 0 || b < 0 || stale < 0 || !(a < b && b < stale) {
		t.Fatalf("managed entries should keep generated order and old-only entries should be last:\n%s", got)
	}
	for _, want := range []string{`custom="keep-a"`, `custom="keep-b"`, `rtp://239.0.0.1:5140`, `rtp://239.0.0.2:5140`} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %s after merge:\n%s", want, got)
		}
	}
}

func TestMergeM3U8OldEntryWithoutTVGIDIsAlwaysOldOnly(t *testing.T) {
	existing := []byte(`#EXTM3U
#EXTINF:-1 tvg-name="同名频道" custom="legacy",同名频道
rtp://239.0.0.99:5140
`)
	generated := []byte(`#EXTM3U
#EXTINF:-1 tvg-id="NEW" tvg-name="同名频道",同名频道
rtp://239.0.0.1:5140
`)

	got := string(MergeM3U8(existing, generated))
	fresh := strings.Index(got, `tvg-id="NEW"`)
	legacy := strings.Index(got, `custom="legacy"`)
	if fresh < 0 || legacy < 0 || fresh > legacy {
		t.Fatalf("old entry without tvg-id should stay unmatched at the end:\n%s", got)
	}
}

func TestMergeM3U8SortsOldEntryWhenDisplayNameIsMissing(t *testing.T) {
	existing := []byte(`#EXTM3U
#EXTINF:-1 tvg-id="CCTV-1" tvg-name="CCTV-1 HD 高清" custom="hd",CCTV-1 HD 高清
rtp://239.0.0.1:5140
#EXTINF:-1 tvg-id="CCTV-1" tvg-name="CCTV-1 4K 超清" custom="4k",CCTV-1 4K 超清
rtp://239.0.0.2:5140
`)
	generated := []byte(`#EXTM3U
#EXTINF:-1 tvg-id="CCTV-1" tvg-name="CCTV-1 HD 高清",CCTV-1 HD 高清
rtp://239.0.0.11:5140
#EXTINF:-1 tvg-id="NEXT" tvg-name="下一个频道",下一个频道
rtp://239.0.0.20:5140
`)

	got := string(MergeM3U8(existing, generated))
	old4k := strings.Index(got, `custom="4k",CCTV-1 4K 超清`)
	hd := strings.Index(got, `rtp://239.0.0.11:5140`)
	next := strings.Index(got, `tvg-id="NEXT"`)
	if old4k < 0 || hd < 0 || next < 0 || !(old4k < hd && hd < next) {
		t.Fatalf("old entry with an existing CommName should participate in DisplayName sorting:\n%s", got)
	}
	if !strings.Contains(got, "rtp://239.0.0.2:5140") {
		t.Fatalf("old DisplayName URL was lost:\n%s", got)
	}
}

func TestMergeM3U8PreservesDuplicateDisplayNameAttrsInOrder(t *testing.T) {
	existing := []byte(`#EXTM3U
#EXTINF:-1 tvg-id="CCTV-1" tvg-name="CCTV-1 HD 高清" custom="first",CCTV-1 HD 高清
rtp://239.0.0.1:5140
#EXTINF:-1 tvg-id="CCTV-1" tvg-name="CCTV-1 HD 高清" custom="second",CCTV-1 HD 高清
rtp://239.0.0.2:5140
#EXTINF:-1 tvg-id="CCTV-1" tvg-name="CCTV-1 HD 高清" custom="legacy",CCTV-1 HD 高清
rtp://239.0.0.3:5140
`)
	generated := []byte(`#EXTM3U
#EXTINF:-1 tvg-id="CCTV-1" tvg-name="CCTV-1 HD 高清",CCTV-1 HD 高清
rtp://239.0.0.11:5140
#EXTINF:-1 tvg-id="CCTV-1" tvg-name="CCTV-1 HD 高清",CCTV-1 HD 高清
rtp://239.0.0.12:5140
`)

	got := string(MergeM3U8(existing, generated))
	first := strings.Index(got, `custom="first"`)
	second := strings.Index(got, `custom="second"`)
	legacy := strings.Index(got, `custom="legacy"`)
	if first < 0 || second < 0 || legacy < 0 || !(first < second && second < legacy) {
		t.Fatalf("duplicate DisplayName entries should keep live URL order:\n%s", got)
	}
	for _, want := range []string{
		`custom="first",CCTV-1 HD 高清` + "\nrtp://239.0.0.11:5140",
		`custom="second",CCTV-1 HD 高清` + "\nrtp://239.0.0.12:5140",
		`custom="legacy",CCTV-1 HD 高清` + "\nrtp://239.0.0.3:5140",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("duplicate DisplayName entries were not merged in order:\n%s", got)
		}
	}
}
