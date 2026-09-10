package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	STB        STBConfig        `yaml:"stb"`
	EPG        EPGConfig        `yaml:"epg"`
	Categories []CategoryConfig `yaml:"categories"`
	Output     OutputConfig     `yaml:"output"`
	Request    RequestConfig    `yaml:"request"`
}

type STBConfig struct {
	UID       string `yaml:"uid"`
	MAC       string `yaml:"mac"`
	SN        string `yaml:"sn"`
	Type      string `yaml:"type"`
	Interface string `yaml:"interface"`
	AuthHost  string `yaml:"auth_host"`
}

type EPGConfig struct {
	Generator   string `yaml:"generator"`
	Source      string `yaml:"source"`
	HistoryDays int    `yaml:"history_days"`
	FutureDays  int    `yaml:"future_days"`
	Timezone    string `yaml:"timezone"`
}

type CategoryConfig struct {
	CateID string `yaml:"cateID"`
	Type   string `yaml:"type"`
}

type OutputConfig struct {
	XML             string `yaml:"xml"`
	M3U8            string `yaml:"m3u8"`
	StreamMode      string `yaml:"stream_mode"`
	FCC             bool   `yaml:"fcc"`
	CatchupDays     int    `yaml:"catchup_days"`
	CatchupTemplate string `yaml:"catchup_template"`
	IncludeShopping bool   `yaml:"include_shopping"`
}

type RequestConfig struct {
	TimeoutSeconds       int `yaml:"timeout_seconds"`
	IntervalMilliseconds int `yaml:"interval_milliseconds"`
}

func Load(path string) (Config, error) {
	var cfg Config
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("read config: %w", err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config: %w", err)
	}
	cfg.defaults()
	if err := cfg.validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func (c *Config) defaults() {
	c.STB.UID = strings.TrimSpace(c.STB.UID)
	c.STB.MAC = strings.TrimSpace(c.STB.MAC)
	c.STB.SN = strings.TrimSpace(c.STB.SN)
	c.STB.Type = strings.TrimSpace(c.STB.Type)
	c.STB.Interface = strings.TrimSpace(c.STB.Interface)
	c.STB.AuthHost = strings.TrimSpace(c.STB.AuthHost)

	c.EPG.Generator = strings.TrimSpace(c.EPG.Generator)
	c.EPG.Source = strings.TrimSpace(c.EPG.Source)
	c.EPG.Timezone = strings.TrimSpace(c.EPG.Timezone)
	if c.EPG.HistoryDays == 0 {
		c.EPG.HistoryDays = 1
	}
	if c.EPG.FutureDays == 0 {
		c.EPG.FutureDays = 3
	}
	if c.EPG.Timezone == "" {
		c.EPG.Timezone = "Asia/Shanghai"
	}

	if len(c.Categories) == 0 {
		c.Categories = defaultCategories()
	}
	for i := range c.Categories {
		c.Categories[i].CateID = strings.TrimSpace(c.Categories[i].CateID)
		c.Categories[i].Type = strings.TrimSpace(c.Categories[i].Type)
	}

	c.Output.XML = strings.TrimSpace(c.Output.XML)
	c.Output.M3U8 = strings.TrimSpace(c.Output.M3U8)
	c.Output.StreamMode = strings.ToLower(strings.TrimSpace(c.Output.StreamMode))
	c.Output.CatchupTemplate = strings.TrimSpace(c.Output.CatchupTemplate)
	if c.Output.XML == "" {
		c.Output.XML = "channels.xml"
	}
	if c.Output.M3U8 == "" {
		c.Output.M3U8 = "channels.m3u8"
	}
	if c.Output.StreamMode == "" {
		c.Output.StreamMode = "rtp"
	}
	if c.Output.CatchupTemplate == "" {
		c.Output.CatchupTemplate = "playseek=${(b)yyyyMMddHHmmss}-${(e)yyyyMMddHHmmss}"
	}

	if c.Request.TimeoutSeconds == 0 {
		c.Request.TimeoutSeconds = 120
	}
}

func (c Config) validate() error {
	if c.STB.UID == "" || c.STB.SN == "" || c.STB.MAC == "" || c.STB.Interface == "" || c.STB.AuthHost == "" {
		return fmt.Errorf("stb.uid, stb.sn, stb.mac, stb.interface and stb.auth_host are required")
	}
	for i, category := range c.Categories {
		if category.CateID == "" {
			return fmt.Errorf("categories[%d].cateID is required", i)
		}
	}
	if c.EPG.HistoryDays < 0 {
		return fmt.Errorf("epg.history_days must be >= 0")
	}
	if c.EPG.FutureDays < 0 {
		return fmt.Errorf("epg.future_days must be >= 0")
	}
	if c.Output.CatchupDays < 0 {
		return fmt.Errorf("output.catchup_days must be >= 0")
	}
	if c.Request.TimeoutSeconds < 0 {
		return fmt.Errorf("request.timeout_seconds must be >= 0")
	}
	if c.Request.IntervalMilliseconds < 0 {
		return fmt.Errorf("request.interval_milliseconds must be >= 0")
	}
	switch c.Output.StreamMode {
	case "raw", "rtp", "udp":
	default:
		return fmt.Errorf("output.stream_mode must be raw, rtp or udp")
	}
	return nil
}

func defaultCategories() []CategoryConfig {
	return []CategoryConfig{{CateID: "000406"}}
}
