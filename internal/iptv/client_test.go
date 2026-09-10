package iptv

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchEPGReturnsErrorWhenAllRequestsFail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
	}))
	defer server.Close()

	client := &Client{epgHostURL: server.URL, http: server.Client()}
	_, err := client.FetchEPG(context.Background(), []ChannelInfo{{MixNo: "10", DisplayName: "CCTV-1"}}, 1, 3, 0)
	if err == nil {
		t.Fatal("expected error when every EPG request fails")
	}
}

func TestFetchEPGReturnsErrorOnPartialFailure(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		if requests == 1 {
			fmt.Fprint(w, `{"data":[]}`)
			return
		}
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
	}))
	defer server.Close()

	client := &Client{epgHostURL: server.URL, http: server.Client()}
	_, err := client.FetchEPG(context.Background(), []ChannelInfo{
		{MixNo: "10", DisplayName: "CCTV-1"},
		{MixNo: "11", DisplayName: "CCTV-2"},
	}, 1, 3, 0)
	if err == nil {
		t.Fatal("expected partial EPG failure to fail the run")
	}
}

func TestFetchEPGAcceptsSuccessfulEmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"data":[]}`)
	}))
	defer server.Close()

	client := &Client{epgHostURL: server.URL, http: server.Client()}
	got, err := client.FetchEPG(context.Background(), []ChannelInfo{{MixNo: "10", DisplayName: "CCTV-1"}}, 1, 3, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("unexpected EPG data: %#v", got)
	}
}

func TestFetchEPGRejectsBusinessError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"errCode":"1001","errMsg":"session expired","data":[]}`)
	}))
	defer server.Close()

	client := &Client{epgHostURL: server.URL, http: server.Client()}
	if _, err := client.FetchEPG(context.Background(), []ChannelInfo{{MixNo: "10", DisplayName: "CCTV-1"}}, 1, 3, 0); err == nil {
		t.Fatal("expected IPTV API error to fail the run")
	}
}

func TestRequestRejectsNon2xxResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotModified)
	}))
	defer server.Close()

	client := &Client{http: server.Client()}
	if _, _, err := client.request(context.Background(), http.MethodGet, server.URL, nil); err == nil {
		t.Fatal("expected non-2xx response to fail")
	}
}

func TestRequestRejectsUnsupportedMethod(t *testing.T) {
	client := &Client{http: http.DefaultClient}
	if _, _, err := client.request(context.Background(), http.MethodPut, "http://example.invalid", nil); err == nil {
		t.Fatal("expected unsupported method to fail before request")
	}
}
