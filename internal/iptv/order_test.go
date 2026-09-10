package iptv

import "testing"

func TestSortChannelInfosDeterministicForSharedMixNo(t *testing.T) {
	infos := []ChannelInfo{
		{MixNo: "10", DisplayName: "频道B HD 高清", IsHD: true},
		{MixNo: "10", DisplayName: "频道A HD 高清", IsHD: true},
		{MixNo: "10", DisplayName: "频道A 4K 超清", IsHD: true, Is4K: true},
		{MixNo: "3", DisplayName: "频道C"},
	}

	sortChannelInfos(infos)
	want := []string{"频道C", "频道A 4K 超清", "频道A HD 高清", "频道B HD 高清"}
	for i := range want {
		if infos[i].DisplayName != want[i] {
			t.Fatalf("[%d] DisplayName=%q, want %q", i, infos[i].DisplayName, want[i])
		}
	}
}
