package main

import (
	"context"
	"flag"
	"log"
	"time"

	"iptv-spider-sh/internal/config"
	"iptv-spider-sh/internal/iptv"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to configuration file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatal(err)
	}
	loc, err := time.LoadLocation(cfg.EPG.Timezone)
	if err != nil {
		log.Fatalf("load timezone %q: %v", cfg.EPG.Timezone, err)
	}

	client, err := iptv.NewClient(cfg.STB, time.Duration(cfg.Request.TimeoutSeconds)*time.Second)
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()

	log.Println("authenticating with Shanghai Telecom IPTV")
	channels, err := client.Authenticate(ctx)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("stream channels discovered: %d", len(channels))

	interval := time.Duration(cfg.Request.IntervalMilliseconds) * time.Millisecond
	infos, err := client.FetchChannelInfosByCategories(ctx, cfg.Categories, interval)
	if err != nil {
		log.Fatal(err)
	}
	infos = iptv.DedupeChannelInfos(infos)
	log.Printf("channels selected: %d", len(infos))

	epgInfos := iptv.DedupeEPGChannelInfos(infos)
	epgData, err := client.FetchEPG(ctx, epgInfos, cfg.EPG.HistoryDays, cfg.EPG.FutureDays, interval)
	if err != nil {
		log.Fatal(err)
	}

	m3u8 := iptv.GenerateM3U8(infos, channels, iptv.M3UOptions{
		StreamMode:      cfg.Output.StreamMode,
		EnableFCC:       cfg.Output.FCC,
		CatchupDays:     cfg.Output.CatchupDays,
		CatchupTemplate: cfg.Output.CatchupTemplate,
		IncludeShopping: cfg.Output.IncludeShopping,
	})
	xmlData, err := iptv.GenerateXMLTV(infos, epgData, iptv.XMLTVOptions{
		Generator:   cfg.EPG.Generator,
		Source:      cfg.EPG.Source,
		HistoryDays: cfg.EPG.HistoryDays,
		Location:    loc,
	})
	if err != nil {
		log.Fatal(err)
	}

	m3uPath, xmlPath, err := iptv.WriteStaticFiles(cfg.Output.M3U8, m3u8, cfg.Output.XML, xmlData)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("generated %s", m3uPath)
	log.Printf("generated %s", xmlPath)
}
