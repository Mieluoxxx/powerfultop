// Command fanhelper is PowerfulTop's root fan daemon. It writes fan keys to the SMC for the one app
// build whose code requirement the installer recorded, and hands the fans back to macOS whenever
// that app stops checking in, the chip overheats, or the daemon stops.
package main

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"powerfultop/internal/thermal"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "PowerfulTop fan helper:", err)
		os.Exit(1)
	}
}

func run() error {
	if os.Getuid() != 0 {
		return fmt.Errorf("must run as root")
	}
	// Refuse every client unless the installer recorded which app build may connect.
	raw, err := os.ReadFile(thermal.ClientRequirementPath)
	requirement := strings.TrimSpace(string(raw))
	if err != nil || requirement == "" {
		return fmt.Errorf("no client requirement")
	}
	smc, err := thermal.OpenSMC()
	if err != nil {
		return err
	}
	server := &thermal.HelperServer{
		Controller:  thermal.NewFanController(smc, thermal.EngagedMarkerPath),
		Requirement: requirement,
	}

	_ = os.Remove(thermal.HelperSocketPath)
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: thermal.HelperSocketPath, Net: "unix"})
	if err != nil {
		return err
	}
	// Anyone may connect; the server checks each peer's code signature before reading a request.
	if err := os.Chmod(thermal.HelperSocketPath, 0o666); err != nil {
		return err
	}

	// launchctl bootout (reinstall, uninstall) sends SIGTERM; without this the fans would stay manual.
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-signals
		server.Shutdown()
		_ = listener.Close()
		_ = os.Remove(thermal.HelperSocketPath)
		os.Exit(0)
	}()

	go func() {
		for range time.Tick(time.Second) {
			server.Tick()
		}
	}()
	return server.Serve(listener)
}
