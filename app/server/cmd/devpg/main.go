package main

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
)

func main() {
	runtimeDir := os.Getenv("ZHIGU_DEVPG_DIR")
	if runtimeDir == "" {
		runtimeDir = filepath.Join(os.TempDir(), "zhigu-devpg")
	}
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir: %v\n", err)
		os.Exit(1)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintf(os.Stderr, "listen: %v\n", err)
		os.Exit(1)
	}
	port := uint32(ln.Addr().(*net.TCPAddr).Port)
	_ = ln.Close()
	cfg := embeddedpostgres.DefaultConfig().
		Username("postgres").
		Password("postgres").
		Database("postgres").
		Version(embeddedpostgres.V16).
		Port(port).
		RuntimePath(runtimeDir).
		Logger(os.Stderr).
		StartTimeout(2 * time.Minute)
	ep := embeddedpostgres.NewDatabase(cfg)
	if err := ep.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "embedded postgres start: %v\n", err)
		os.Exit(1)
	}
	dsn := fmt.Sprintf("host=127.0.0.1 port=%d user=postgres password=postgres dbname=postgres sslmode=disable TimeZone=UTC", port)
	fmt.Println(dsn)
	if out := os.Getenv("ZHIGU_DEVPG_DSN_FILE"); out != "" {
		_ = os.WriteFile(out, []byte(dsn+"\n"), 0o600)
	}
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	_ = ep.Stop()
}
