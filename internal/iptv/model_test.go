package iptv

import "testing"

func TestDedupeChannelInfosKeepsHDAnd4KDropsSD(t *testing.T) {
	in := []ChannelInfo{
		normalizeChannelInfo(ChannelInfo{Name: "CCTV-1", MixNo: "10"}),
		normalizeChannelInfo(ChannelInfo{Name: "CCTV-1 HD", MixNo: "10"}),
		normalizeChannelInfo(ChannelInfo{Name: "CCTV-1-4K", MixNo: "10"}),
		normalizeChannelInfo(ChannelInfo{Name: "东方卫视", MixNo: "3"}),
	}

	got := DedupeChannelInfos(in)
	if len(got) != 3 {
		t.Fatalf("len=%d, want 3: %#v", len(got), got)
	}
	wantMix := []string{"3", "10", "10"}
	wantDisplay := []string{"东方卫视", "CCTV-1 4K 超清", "CCTV-1 HD 高清"}
	for i := range got {
		if got[i].MixNo != wantMix[i] {
			t.Errorf("[%d] MixNo=%q, want %q", i, got[i].MixNo, wantMix[i])
		}
		if got[i].DisplayName != wantDisplay[i] {
			t.Errorf("[%d] DisplayName=%q, want %q", i, got[i].DisplayName, wantDisplay[i])
		}
	}
	if got[1].CommName != "CCTV-1" || got[2].CommName != "CCTV-1" {
		t.Fatalf("quality variants do not share CommName: %#v", got)
	}
}

func TestDedupeChannelInfosEnhancedVariantCoversSD(t *testing.T) {
	got := DedupeChannelInfos([]ChannelInfo{
		normalizeChannelInfo(ChannelInfo{Name: "频道A", MixNo: "20"}),
		normalizeChannelInfo(ChannelInfo{Name: "频道A 4K", MixNo: "20"}),
		normalizeChannelInfo(ChannelInfo{Name: "频道B", MixNo: "30"}),
		normalizeChannelInfo(ChannelInfo{Name: "频道B HD", MixNo: "30"}),
	})
	if len(got) != 2 {
		t.Fatalf("len=%d, want 2: %#v", len(got), got)
	}
	if got[0].MixNo != "20" || got[0].CommName != "频道A" || !got[0].Is4K {
		t.Errorf("channel A=%#v", got[0])
	}
	if got[1].MixNo != "30" || got[1].CommName != "频道B" || !got[1].IsHD || got[1].Is4K {
		t.Errorf("channel B=%#v", got[1])
	}
}

func TestDedupeChannelInfosIgnoresIsCharge(t *testing.T) {
	got := DedupeChannelInfos([]ChannelInfo{
		normalizeChannelInfo(ChannelInfo{Name: "频道A HD", MixNo: "10", IsCharge: "1"}),
		normalizeChannelInfo(ChannelInfo{Name: "频道A HD", MixNo: "10", IsCharge: "0"}),
	})
	if len(got) != 1 || got[0].IsCharge != "1" {
		t.Fatalf("IsCharge affected selection: %#v", got)
	}
}

func TestDedupeEPGChannelInfosKeepsOnePerMixNo(t *testing.T) {
	in := []ChannelInfo{
		{MixNo: "10", CommName: "CCTV-1", DisplayName: "CCTV-1 HD 高清"},
		{MixNo: "3", CommName: "东方卫视", DisplayName: "东方卫视"},
		{MixNo: "10", CommName: "CCTV-1", DisplayName: "CCTV-1 4K 超清"},
	}
	got := DedupeEPGChannelInfos(in)
	if len(got) != 2 {
		t.Fatalf("len=%d, want 2: %#v", len(got), got)
	}
	if got[0].MixNo != "3" || got[1].MixNo != "10" {
		t.Fatalf("unexpected EPG channels: %#v", got)
	}
	if got[1].DisplayName != "CCTV-1 HD 高清" {
		t.Fatalf("EPG dedupe should keep the first variant for the shared MixNo: %#v", got[1])
	}
}
