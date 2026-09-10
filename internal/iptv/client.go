package iptv

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"iptv-spider-sh/internal/config"

	"github.com/PuerkitoBio/goquery"
	"github.com/robertkrimen/otto"
)

const userAgent = "webkit;Resolution(PAL,720P,1080P,2160P,4K)"

var (
	locationPattern    = regexp.MustCompile(`top\.document\.location\s*=\s*['\"]([^'\"]+)['\"]`)
	jsSetConfigPattern = regexp.MustCompile(`jsSetConfig\s*\(\s*['\"]([^'\"]+)['\"]\s*,\s*['\"]([^'\"]*)['\"]\s*\)`)
)

type Client struct {
	stb        config.STBConfig
	ip         string
	http       *http.Client
	epgHostURL string
}

func NewClient(stb config.STBConfig, timeout time.Duration) (*Client, error) {
	ip, err := interfaceIPv4(stb.Interface)
	if err != nil {
		return nil, err
	}
	dialer, err := interfaceDialer(stb.Interface, ip)
	if err != nil {
		return nil, err
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = dialer.DialContext
	return &Client{
		stb: stb,
		ip:  ip.String(),
		http: &http.Client{
			Jar:       jar,
			Timeout:   timeout,
			Transport: transport,
		},
	}, nil
}

func interfaceIPv4(name string) (net.IP, error) {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return nil, fmt.Errorf("find network interface %q: %w", name, err)
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return nil, fmt.Errorf("list addresses for network interface %q: %w", name, err)
	}
	for _, addr := range addrs {
		ip, _, err := net.ParseCIDR(addr.String())
		if err == nil {
			if ip = ip.To4(); ip != nil {
				return ip, nil
			}
		}
	}
	return nil, fmt.Errorf("network interface %q has no IPv4 address", name)
}

func (c *Client) Authenticate(ctx context.Context) ([]Channel, error) {
	doc, err := c.pre4kLogAuth(ctx)
	if err != nil {
		return nil, err
	}
	doc, err = c.r4kLogAuth(ctx, doc)
	if err != nil {
		return nil, err
	}
	doc, err = c.ottAuth(ctx, doc)
	if err != nil {
		return nil, err
	}
	channels, err := processChannels(doc)
	if err != nil {
		return nil, err
	}

	doc, err = c.epgIndex(ctx, doc)
	if err != nil {
		return nil, err
	}
	doc, err = c.epgLoadBalance(ctx, doc)
	if err != nil {
		return nil, err
	}
	if err := c.epgPortalAuth(ctx, doc); err != nil {
		return nil, err
	}
	if err := c.epgGetPortal(ctx); err != nil {
		return nil, err
	}
	return channels, nil
}

func (c *Client) pre4kLogAuth(ctx context.Context) (*goquery.Document, error) {
	u := fmt.Sprintf("http://%s/iptv3a/4kLogAuth.do", c.stb.AuthHost)
	body, finalURL, err := c.request(ctx, http.MethodGet, u, map[string]string{
		"Action":     "Login",
		"UserID":     c.stb.UID,
		"SN":         c.stb.SN,
		"Type":       "iptv4k",
		"Mode":       "MENU.SMG-4K",
		"FCCSupport": "1",
	})
	if err != nil {
		return nil, fmt.Errorf("pre-auth request: %w", err)
	}
	return document(finalURL, body)
}

func (c *Client) r4kLogAuth(ctx context.Context, doc *goquery.Document) (*goquery.Document, error) {
	action, method, form, err := formParams(doc, "form")
	if err != nil {
		return nil, fmt.Errorf("r4k auth form: %w", err)
	}
	body, finalURL, err := c.request(ctx, method, action, form)
	if err != nil {
		return nil, fmt.Errorf("r4k auth request: %w", err)
	}
	return document(finalURL, body)
}

func (c *Client) ottAuth(ctx context.Context, doc *goquery.Document) (*goquery.Document, error) {
	vm := otto.New()
	runDocumentScripts(vm, doc)
	value, err := vm.Get("encrytoken")
	if err != nil || !value.IsDefined() {
		return nil, fmt.Errorf("encrytoken not found in auth page")
	}
	authenticator, err := newAuthenticator(value.String(), c.stb.UID, c.stb.SN, c.ip, c.stb.MAC).encryptedString()
	if err != nil {
		return nil, fmt.Errorf("build authenticator: %w", err)
	}
	action, method, form, err := formParams(doc, "form")
	if err != nil {
		return nil, fmt.Errorf("OTT auth form: %w", err)
	}
	form["authenticator"] = authenticator
	body, finalURL, err := c.request(ctx, method, action, form)
	if err != nil {
		return nil, fmt.Errorf("OTT auth request: %w", err)
	}
	return document(finalURL, body)
}

func (c *Client) epgIndex(ctx context.Context, doc *goquery.Document) (*goquery.Document, error) {
	action, method, form, err := formParams(doc, "form#epgform")
	if err != nil {
		return nil, fmt.Errorf("EPG form: %w", err)
	}
	body, finalURL, err := c.request(ctx, method, action, form)
	if err != nil {
		return nil, fmt.Errorf("EPG index request: %w", err)
	}
	return document(finalURL, body)
}

func (c *Client) epgLoadBalance(ctx context.Context, doc *goquery.Document) (*goquery.Document, error) {
	for _, script := range documentScripts(doc) {
		match := locationPattern.FindStringSubmatch(script)
		if len(match) != 2 {
			continue
		}
		u, err := doc.Url.Parse(match[1])
		if err != nil {
			return nil, fmt.Errorf("parse EPG load-balance URL: %w", err)
		}
		body, finalURL, err := c.request(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, fmt.Errorf("EPG load-balance request: %w", err)
		}
		return document(finalURL, body)
	}
	return nil, fmt.Errorf("EPG load-balance URL not found")
}

func (c *Client) epgPortalAuth(ctx context.Context, doc *goquery.Document) error {
	action, method, form, err := formParams(doc, "form")
	if err != nil {
		return fmt.Errorf("EPG portal auth form: %w", err)
	}
	userToken := form["UserToken"]
	if userToken == "" {
		return fmt.Errorf("EPG portal auth form does not contain UserToken")
	}
	stbInfo, err := privateEncryptToken(userToken)
	if err != nil {
		return fmt.Errorf("build stbinfo: %w", err)
	}
	form["stbtype"] = c.stb.Type
	form["stbinfo"] = stbInfo
	body, finalURL, err := c.request(ctx, method, action, form)
	if err != nil {
		return fmt.Errorf("EPG portal auth request: %w", err)
	}
	respDoc, err := document(finalURL, body)
	if err != nil {
		return err
	}
	info := parseJSConfig(respDoc)
	if info["SessionID"] == "" || info["IpPort"] == "" || info["framecode"] == "" {
		return fmt.Errorf("EPG portal auth response misses SessionID/IpPort/framecode")
	}
	c.epgHostURL = fmt.Sprintf("http://%s/iptvepg/%s", info["IpPort"], info["framecode"])
	cookieURL, err := url.Parse(c.epgHostURL)
	if err != nil {
		return err
	}
	c.http.Jar.SetCookies(cookieURL, []*http.Cookie{{
		Name:     "JSESSIONID",
		Value:    info["SessionID"],
		Path:     "/",
		HttpOnly: true,
	}})
	return nil
}

func (c *Client) epgGetPortal(ctx context.Context) error {
	if c.epgHostURL == "" {
		return fmt.Errorf("EPG host URL is empty")
	}
	_, _, err := c.request(ctx, http.MethodGet, c.epgHostURL+"/portal.jsp", nil)
	if err != nil {
		return fmt.Errorf("open EPG portal: %w", err)
	}
	return nil
}

func (c *Client) FetchEPG(ctx context.Context, infos []ChannelInfo, historyDays, futureDays int, interval time.Duration) (map[string][]EPGDetails, error) {
	if c.epgHostURL == "" {
		return nil, fmt.Errorf("client is not authenticated")
	}
	result := make(map[string][]EPGDetails, len(infos))
	endpoint := c.epgHostURL + "/function/ajax/epg7getChannelByAjax.jsp"
	now := time.Now()
	start := now.AddDate(0, 0, -historyDays).UnixMilli()
	end := now.AddDate(0, 0, futureDays).UnixMilli()

	for i, ch := range infos {
		body, _, err := c.request(ctx, http.MethodPost, endpoint, map[string]string{
			"action":    "getChannelProg",
			"code":      ch.Code,
			"channelID": ch.ChID,
			"endTime":   strconv.FormatInt(end, 10),
			"startTime": strconv.FormatInt(start, 10),
			"offset":    "0",
			"limit":     "2000",
		})
		if err != nil {
			return nil, fmt.Errorf("EPG %s: %w", ch.CommName, err)
		}
		var resp jsonResponse[EPGDetails]
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("decode EPG %s: %w", ch.CommName, err)
		}
		const epgNoProgrammeCode = "051002"
		if resp.ErrCode == epgNoProgrammeCode {
			log.Printf("EPG %-20s %s", ch.CommName, resp.ErrMsg)
			continue
		}
		if err := resp.err(); err != nil {
			return nil, fmt.Errorf("EPG %s: %w", ch.CommName, err)
		}
		if len(resp.Data) > 0 {
			result[ch.MixNo] = resp.Data
		}
		log.Printf("EPG %-20s %d programmes", ch.CommName, len(resp.Data))
		if i < len(infos)-1 && interval > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(interval):
			}
		}
	}
	return result, nil
}

func (c *Client) request(ctx context.Context, method, rawURL string, form map[string]string) ([]byte, *url.URL, error) {
	method = strings.ToUpper(strings.TrimSpace(method))
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, nil, err
	}

	var body io.Reader
	switch method {
	case http.MethodGet:
		if len(form) > 0 {
			q := u.Query()
			for k, v := range form {
				q.Set(k, v)
			}
			u.RawQuery = q.Encode()
		}
	case http.MethodPost:
		values := url.Values{}
		for k, v := range form {
			values.Set(k, v)
		}
		body = strings.NewReader(values.Encode())
	default:
		return nil, nil, fmt.Errorf("unsupported HTTP method %q", method)
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.Request.URL, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return data, resp.Request.URL, fmt.Errorf("HTTP %s", resp.Status)
	}
	return data, resp.Request.URL, nil
}

func document(source *url.URL, body []byte) (*goquery.Document, error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	doc.Url = source
	return doc, nil
}

func formParams(doc *goquery.Document, selector string) (string, string, map[string]string, error) {
	forms := doc.Find(selector)
	if forms.Length() != 1 {
		return "", "", nil, fmt.Errorf("expected one %q form, got %d", selector, forms.Length())
	}
	form := forms.First()
	action := strings.TrimSpace(form.AttrOr("action", ""))
	if action == "" {
		action = doc.Url.String()
	} else {
		rel, err := url.Parse(action)
		if err != nil {
			return "", "", nil, err
		}
		action = doc.Url.ResolveReference(rel).String()
	}
	method := strings.ToUpper(form.AttrOr("method", http.MethodGet))
	params := make(map[string]string)
	form.Find("input[name]").Each(func(_ int, s *goquery.Selection) {
		params[s.AttrOr("name", "")] = s.AttrOr("value", "")
	})
	form.Find("textarea[name]").Each(func(_ int, s *goquery.Selection) {
		params[s.AttrOr("name", "")] = s.Text()
	})
	form.Find("select[name]").Each(func(_ int, s *goquery.Selection) {
		option := s.Find("option[selected]").First()
		if option.Length() == 0 {
			option = s.Find("option").First()
		}
		params[s.AttrOr("name", "")] = option.AttrOr("value", option.Text())
	})
	delete(params, "")
	return action, method, params, nil
}

func documentScripts(doc *goquery.Document) []string {
	var scripts []string
	doc.Find("script").Each(func(_ int, s *goquery.Selection) {
		if text := strings.TrimSpace(s.Text()); text != "" {
			scripts = append(scripts, text)
		}
	})
	return scripts
}

func runDocumentScripts(vm *otto.Otto, doc *goquery.Document) {
	for _, script := range documentScripts(doc) {
		_, _ = vm.Run(script)
	}
}

func processChannels(doc *goquery.Document) ([]Channel, error) {
	vm := otto.New()
	runDocumentScripts(vm, doc)
	channels := make([]Channel, 0, 128)
	if err := vm.Set("AddChannel", func(call otto.FunctionCall) otto.Value {
		channels = append(channels, parseChannel(call.Argument(0).String()))
		return otto.Value{}
	}); err != nil {
		return nil, err
	}
	if _, err := vm.Run(`for (var i = 0; i < channelArray.length; i++) { AddChannel(channelArray[i]); }`); err != nil {
		return nil, fmt.Errorf("read channelArray: %w", err)
	}
	if len(channels) == 0 {
		return nil, fmt.Errorf("channelArray is empty")
	}
	return channels, nil
}

func parseChannel(raw string) Channel {
	values := make(map[string]string)
	for _, part := range strings.Split(raw, ",") {
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		values[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"`)
	}
	return Channel{
		UserChannelID:  values["UserChannelID"],
		ChannelURL:     values["ChannelURL"],
		TimeShiftURL:   values["TimeShiftURL"],
		ChannelFCCPort: values["ChannelFCCPort"],
		ChannelFCCIP:   values["ChannelFCCIP"],
	}
}

func parseJSConfig(doc *goquery.Document) map[string]string {
	out := make(map[string]string)
	for _, script := range documentScripts(doc) {
		for _, match := range jsSetConfigPattern.FindAllStringSubmatch(script, -1) {
			if len(match) == 3 {
				out[match[1]] = match[2]
			}
		}
	}
	return out
}
