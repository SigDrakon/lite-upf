// Command lite-upf runs the lightweight 5G User Plane Function.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/SigDrakon/lite-upf/internal/config"
	"github.com/SigDrakon/lite-upf/internal/gtpu"
	"github.com/SigDrakon/lite-upf/internal/pfcp"
	"github.com/SigDrakon/lite-upf/internal/version"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "lite-upf: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", config.DefaultPath, "path to the configuration file")
	showVersion := flag.Bool("version", false, "print version information and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version.String())
		return nil
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	log := newLogger(cfg.Logging.Level)
	log.Info("starting", "version", version.String())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// N4 / PFCP control plane.
	var pfcpServer *pfcp.Server
	if cfg.Features.PFCP {
		pfcpServer, err = pfcp.NewServer(cfg.N4.Listen, log)
		if err != nil {
			return err
		}
		log.Info("n4: listening", "interface", "N4", "addr", pfcpServer.Addr().String())
		go func() {
			if err := pfcpServer.Serve(); err != nil && !errors.Is(err, net.ErrClosed) {
				log.Error("n4: serve stopped", "error", err.Error())
			}
		}()
	}

	// N3 / GTP-U access plane.
	var gtpuServer *gtpu.Server
	if cfg.Features.GTPU {
		gtpuServer, err = gtpu.NewServer(cfg.N3.Listen, log)
		if err != nil {
			return err
		}
		log.Info("n3: listening", "interface", "N3", "addr", gtpuServer.Addr().String())
		go func() {
			if err := gtpuServer.Serve(); err != nil && !errors.Is(err, net.ErrClosed) {
				log.Error("n3: serve stopped", "error", err.Error())
			}
		}()
	}

	log.Info("n6: forwarding interface", "interface", "N6", "name", cfg.N6.Interface)
	log.Info("ready")

	<-ctx.Done()
	log.Info("shutting down")

	if gtpuServer != nil {
		_ = gtpuServer.Close()
	}
	if pfcpServer != nil {
		_ = pfcpServer.Close()
	}
	return nil
}

// newLogger builds the process logger at the configured level.
func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch level {
	case "trace", "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl}))
}
