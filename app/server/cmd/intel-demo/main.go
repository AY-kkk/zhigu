package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	intelapi "zhigu/server/api/v1/intel"
	"zhigu/server/initialize"
	intelsvc "zhigu/server/service/intel"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	runtimeDir, err := os.MkdirTemp("", "zhigu-intel-demo-pg-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(runtimeDir)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	port := uint32(ln.Addr().(*net.TCPAddr).Port)
	_ = ln.Close()
	pg := embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().
		Username("postgres").
		Password("postgres").
		Database("postgres").
		Version(embeddedpostgres.V16).
		Port(port).
		RuntimePath(runtimeDir).
		Logger(io.Discard).
		StartTimeout(90 * time.Second))
	if err := pg.Start(); err != nil {
		return fmt.Errorf("start embedded postgres: %w", err)
	}
	defer pg.Stop()
	dsn := fmt.Sprintf("host=127.0.0.1 port=%d user=postgres password=postgres dbname=postgres sslmode=disable TimeZone=UTC", port)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		return err
	}
	if err := initialize.Migrate(db); err != nil {
		return err
	}
	fixtureDir := os.Getenv("ZHIGU_INTEL_FIXTURE_DIR")
	if fixtureDir == "" {
		fixtureDir = "../../contracts/intel/fixtures/events-v1"
	}
	svc := intelsvc.NewService(db, []byte(os.Getenv("ZHIGU_INTEL_COOKIE_SECRET")), fixtureDir)
	engine := gin.New()
	engine.Use(gin.Recovery())
	engine.GET("/healthz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok", "module": "intel-demo"}) })
	intelapi.Register(engine, svc)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	svc.StartWorker(ctx)
	addr := os.Getenv("ZHIGU_HTTP_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8099"
	}
	server := &http.Server{Addr: addr, Handler: engine}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	fmt.Println("intel-demo listening on", addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}
