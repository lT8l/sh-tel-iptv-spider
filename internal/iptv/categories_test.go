package iptv

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"iptv-spider-sh/internal/config"
)

func TestFetchChannelInfosKeepsQualityVariantsWithSameMixNo(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":[
			{"name":"CCTV-1","mixNo":"10"},
			{"name":"CCTV-1 HD","mixNo":"10"},
			{"name":"CCTV-1-4K","mixNo":"10"}
		]}`)
	}))
	defer testServer.Close()

	client := &Client{epgHostURL: testServer.URL, http: testServer.Client()}
	infos, err := client.FetchChannelInfosByCategories(context.Background(), []config.CategoryConfig{{CateID: "000406"}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 3 {
		t.Fatalf("same-MixNo quality variants collapsed before dedupe: %#v", infos)
	}

	selected := DedupeChannelInfos(infos)
	if len(selected) != 2 || !selected[0].Is4K || !selected[1].IsHD || selected[0].MixNo != "10" || selected[1].MixNo != "10" {
		t.Fatalf("unexpected selected variants: %#v", selected)
	}
}

func TestFetchChannelInfosReturnsErrorOnPartialFailure(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if r.Form.Get("cateID") == "broken" {
			http.Error(w, "upstream unavailable", http.StatusBadGateway)
			return
		}
		fmt.Fprint(w, `{"data":[{"name":"CCTV-1 HD","mixNo":"10"}]}`)
	}))
	defer testServer.Close()

	client := &Client{epgHostURL: testServer.URL, http: testServer.Client()}
	_, err := client.FetchChannelInfosByCategories(context.Background(), []config.CategoryConfig{
		{CateID: "000406"},
		{CateID: "broken"},
	}, 0)
	if err == nil {
		t.Fatal("expected partial category failure to fail the run")
	}
}

func TestFetchChannelInfosRejectsBusinessError(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"errCode":"1001","errMsg":"session expired","data":[]}`)
	}))
	defer testServer.Close()

	client := &Client{epgHostURL: testServer.URL, http: testServer.Client()}
	if _, err := client.FetchChannelInfosByCategories(context.Background(), []config.CategoryConfig{{CateID: "000406"}}, 0); err == nil {
		t.Fatal("expected IPTV API error to fail the run")
	}
}
