package config

import (
	"os"
	"path/filepath"
	"testing"
)

const testSTBConfig = `stb:
  uid: user
  mac: aa:bb:cc:dd:ee:ff
  sn: serial
  interface: eth1
  auth_host: 127.0.0.1:7001
`

func TestLoadAllowsZeroRequestInterval(t *testing.T) {
	path := writeTestConfig(t, testSTBConfig+`request:
  interval_milliseconds: 0
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Request.IntervalMilliseconds != 0 {
		t.Fatalf("interval=%d, want 0", cfg.Request.IntervalMilliseconds)
	}
}

func TestLoadRejectsNegativeDurations(t *testing.T) {
	for name, config := range map[string]string{
		"history":  "epg:\n  history_days: -1\n",
		"future":   "epg:\n  future_days: -1\n",
		"timeout":  "request:\n  timeout_seconds: -1\n",
		"interval": "request:\n  interval_milliseconds: -1\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Load(writeTestConfig(t, testSTBConfig+config)); err == nil {
				t.Fatal("expected negative value to fail")
			}
		})
	}
}

func TestLoadDoesNotClampHistoryDays(t *testing.T) {
	path := writeTestConfig(t, testSTBConfig+`epg:
  history_days: 14
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.EPG.HistoryDays != 14 {
		t.Fatalf("history_days=%d, want 14", cfg.EPG.HistoryDays)
	}
}

func TestLoadNormalizesStreamMode(t *testing.T) {
	path := writeTestConfig(t, testSTBConfig+`output:
  stream_mode: ' RTP '
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Output.StreamMode != "rtp" {
		t.Fatalf("stream_mode=%q, want rtp", cfg.Output.StreamMode)
	}
}

func writeTestConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
