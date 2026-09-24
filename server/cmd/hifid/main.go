// Command hifid is the control service of HiFi-NetEase-Server: it adapts MPD
// to a REST + WebSocket API and serves the mobile PWA (docs/03, docs/09).
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Zulolo/HiFi-NetEase-Server/server/internal/api"
	"github.com/Zulolo/HiFi-NetEase-Server/server/internal/config"
	"github.com/Zulolo/HiFi-NetEase-Server/server/internal/netease"
	"github.com/Zulolo/HiFi-NetEase-Server/server/internal/player"
	"github.com/Zulolo/HiFi-NetEase-Server/server/internal/web"
)

// version is overridden at build time with -ldflags "-X main.version=..."
var version = "dev"

func main() {
	cfgPath := flag.String("config", "/etc/hifid/config.yaml", "path to config.yaml")
	listen := flag.String("listen", "", "override listen address (host:port)")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		os.Stdout.WriteString("hifid " + version + "\n")
		return
	}

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Error("config", "err", err)
		os.Exit(1)
	}
	if *listen != "" {
		cfg.Listen = *listen
	}

	network, addr := cfg.MPD.Dial()
	pl := player.New(network, addr)
	defer pl.Close()
	log.Info("mpd target", "network", network, "addr", addr)

	srv := api.New(cfg, pl, log, version)

	// NetEase adapter (ADR-0002). A failure here must not stop local playback
	// (NFR-6): hifid logs it and serves everything else.
	if cfg.NetEase.Enabled {
		ncm, err := netease.New(netease.Config{
			StateDir: filepath.Join(cfg.Paths.Data, "netease"),
			Levels:   cfg.NetEase.LevelPreference,
		})
		if err != nil {
			log.Error("netease adapter unavailable", "err", err)
		} else {
			srv.SetNetEase(ncm)
			if p, err := ncm.Profile(context.Background()); err != nil {
				log.Warn("netease session not usable yet", "err", err)
			} else {
				log.Info("netease session", "nickname", p.Nickname, "vip_type", p.VipType)
			}
		}
	}

	// Fan MPD idle events out to WebSocket clients. A missing MPD at start-up
	// is not fatal: hifid must serve status so the UI can show the problem.
	stopWatch, err := pl.Watch(func(subsystem string) {
		srv.Broadcast(subsystem)
	})
	if err != nil {
		log.Warn("mpd watcher unavailable, continuing without live events", "err", err)
	} else {
		defer stopWatch()
	}

	httpSrv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           srv.Routes(web.Handler()),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Info("listening", "addr", cfg.Listen, "auth", cfg.Auth.Mode, "version", version)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("http", "err", err)
			os.Exit(1)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Info("shutting down")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(ctx)
}
