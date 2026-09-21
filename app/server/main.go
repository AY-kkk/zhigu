package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"

	financeapi "zhigu/server/api/v1/finance"
	"zhigu/server/httpx"
	"zhigu/server/initialize"
	"zhigu/server/service/finance"
	"zhigu/server/service/market"
	"zhigu/server/service/workbench"
)

func main() {
	addr := os.Getenv("ZHIGU_HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	db, err := initialize.OpenDB()
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	if err := initialize.Migrate(db); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	if err := initialize.Seed(db); err != nil {
		log.Fatalf("seed: %v", err)
	}
	budget := finance.NewDBBudget(db)
	client, err := finance.NewHTTPResearchClient()
	if err != nil {
		log.Fatalf("research client: %v", err)
	}
	svc := finance.NewService(db, client, budget, finance.NewFixtureConfig())
	cfg := finance.NewConfigService(db)
	proxy := finance.NewModelProxy(db, budget)
	ctx := context.Background()
	svc.StartWorker(ctx)
	engine := gin.New()
	engine.Use(gin.Recovery())
	engine.GET("/healthz", func(c *gin.Context) {
		mode := "http"
		if os.Getenv("ZHIGU_RESEARCH_MODE") == "unit-test" {
			mode = "fake"
		}
		httpx.OK(c, http.StatusOK, gin.H{
			"status": "ok", "stage": "B", "mode": finance.DataMode(),
			"research_client": mode, "research_url_configured": os.Getenv("ZHIGU_RESEARCH_URL") != "",
			"catalog_size": finance.CatalogSize(),
		})
	})
	financeapi.Register(engine, svc, proxy)
	financeapi.RegisterAdmin(engine, cfg, svc)
	mkt := market.NewService(db)
	hub := workbench.NewHub(db, mkt).UseConfig(cfg)
	financeapi.RegisterStrategy(engine, hub)
	if mkt.Mode() == "live" {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			if err := mkt.SyncLive(ctx); err != nil {
				log.Printf("market catalog sync: %v", err)
			} else {
				log.Printf("market catalog synced")
			}
		}()
	}
	_ = initialize.RegisterFinanceModels(db)
	initialize.RegisterFinanceRouter()
	log.Printf("zhigu-server listening on %s", addr)
	if err := engine.Run(addr); err != nil {
		log.Fatal(err)
	}
}
