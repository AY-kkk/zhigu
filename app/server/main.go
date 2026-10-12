package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"

	financeapi "zhigu/server/api/v1/finance"
	futuresapi "zhigu/server/api/v1/futures"
	intelapi "zhigu/server/api/v1/intel"
	"zhigu/server/httpx"
	"zhigu/server/initialize"
	"zhigu/server/runtime"
	"zhigu/server/service/finance"
	futuressvc "zhigu/server/service/futures"
	intelsvc "zhigu/server/service/intel"
	"zhigu/server/service/market"
	strategymarket "zhigu/server/service/strategy_market"
	"zhigu/server/service/workbench"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--futures-extract-v1" {
		os.Exit(futuressvc.RunFuturesExtractionCLI())
	}
	runtimeConfig, err := runtime.LoadEnv()
	if err != nil {
		log.Fatalf("runtime configuration: %v", err)
	}
	if err := runtimeConfig.ApplyEnvironment(); err != nil {
		log.Fatalf("runtime configuration: %v", err)
	}
	log.Printf("runtime configuration: %s", runtimeConfig.SummaryJSON())
	addr := runtimeConfig.HTTPAddr
	db, err := initialize.OpenDBWithDSN(runtimeConfig.DatabaseDSN)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	if err := initialize.Migrate(db); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	if err := initialize.EnsureOwner(db, runtimeConfig); err != nil {
		log.Fatalf("owner: %v", err)
	}
	if runtimeConfig.SeedDemoUsers {
		if err := initialize.Seed(db); err != nil {
			log.Fatalf("seed: %v", err)
		}
	}
	futuresDB, futuresDBErr := initialize.OpenDBWithDSN(runtimeConfig.DatabaseDSN)
	if futuresDBErr != nil {
		log.Printf("futures database unavailable: %v", futuresDBErr)
	}
	futuresEnabled := runtimeConfig.Futures.Enabled
	futuresState := futuressvc.RuntimeState{Enabled: futuresEnabled, Ready: false, Mode: "off", Reason: "FUTURES_NOT_IMPLEMENTED"}
	var futuresDomain *futuressvc.Domain
	if futuresDBErr == nil {
		if sqlDB, err := futuresDB.DB(); err == nil {
			sqlDB.SetMaxOpenConns(4)
			sqlDB.SetMaxIdleConns(2)
		}
		state, err := futuressvc.Bootstrap(context.Background(), futuresDB, futuressvc.Config{Enabled: futuresEnabled, Mode: string(runtimeConfig.Futures.OperationMode)})
		if err != nil {
			log.Printf("futures bootstrap failed; module remains isolated: %v", err)
		}
		futuresState = state
		if err == nil && futuressvc.MaintenanceAvailable(futuresDB) {
			futuresDomain = futuressvc.NewDomain(futuresDB, os.Getenv("ZHIGU_FUTURES_STORAGE_DIR")).WithMode(futuresState.Mode).WithEnabled(futuresEnabled)
			go futuressvc.StartFuturesMaintenance(context.Background(), futuresDB, futuresDomain)
		}
	}
	budget := finance.NewDBBudget(db)
	var client finance.ResearchClient
	var fixtureConfig *finance.FixtureConfig
	if runtimeConfig.DeploymentEnvironment == runtime.Production {
		client = &finance.HTTPResearchClient{Base: os.Getenv("ZHIGU_RESEARCH_URL"), Token: runtimeConfig.InternalToken, Client: &http.Client{Timeout: 10 * time.Second}}
	} else {
		client, err = finance.NewHTTPResearchClient()
		if err != nil {
			log.Fatalf("research client: %v", err)
		}
		fixtureConfig = finance.NewFixtureConfig()
	}
	svc := finance.NewService(db, client, budget, fixtureConfig)
	cfg := finance.NewConfigService(db)
	proxy := finance.NewModelProxy(db, budget)
	ctx := context.Background()
	if runtimeConfig.DeploymentEnvironment != runtime.Production || (runtimeConfig.Research.Enabled && runtimeConfig.Research.OperationMode == runtime.Live) {
		svc.StartWorker(ctx)
	}
	engine := gin.New()
	engine.Use(gin.Recovery())
	engine.Use(func(c *gin.Context) {
		if !runtimeConfig.RequestAllowed(c.Request.Method, c.Request.URL.Path, c.GetHeader("X-Intel-Mode")) {
			httpx.Fail(c, http.StatusServiceUnavailable, "RUNTIME_UNAVAILABLE", "当前模块暂不可用")
			c.Abort()
			return
		}
		if runtimeConfig.DeploymentEnvironment == runtime.Production && runtimeConfig.Intel.Enabled {
			c.Request.Header.Set("X-Intel-Mode", "live")
		}
		c.Next()
	})
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
	futuresapi.Register(engine, futuressvc.NewService(futuresEnabled).WithRuntimeState(futuresState).WithDomain(futuresDomain))
	if futuresDomain != nil {
		futuresGrants := futuressvc.NewGrantManager(futuressvc.NewDBGrantStore(futuresDB), futuressvc.DBLeaseAuthorizer{DB: futuresDB}, nil)
		futuresapi.RegisterInternal(engine, futuresGrants, futuresDomain)
		if futuresState.Ready && futuresState.Mode == "live" {
			go futuressvc.StartRunScheduler(context.Background(), futuresDB, futuresDomain, futuresGrants)
		}
	}
	mkt := market.NewService(db)
	hub := workbench.NewHub(db, mkt).UseConfig(cfg)
	financeapi.RegisterStrategy(engine, hub)
	financeapi.RegisterStrategyMarket(engine, strategymarket.New(db, mkt))
	if runtimeConfig.Intel.Enabled {
		cookieSecret := runtimeConfig.IntelCookieSecret
		if cookieSecret == "" {
			log.Fatal("ZHIGU_INTEL_COOKIE_SECRET is required when ZHIGU_INTEL_ENABLED=true")
		}
		if os.Getenv("ZHIGU_INTEL_MODE") == "live" {
			if os.Getenv("ZHIGU_JWT_SECRET") == "" {
				log.Fatal("ZHIGU_JWT_SECRET is required for ZHIGU_INTEL_MODE=live")
			}
			if os.Getenv("ZHIGU_INTEL_PUBLIC_ORIGIN") == "" {
				log.Fatal("ZHIGU_INTEL_PUBLIC_ORIGIN is required for ZHIGU_INTEL_MODE=live")
			}
		}
		fixtureDir := os.Getenv("ZHIGU_INTEL_FIXTURE_DIR")
		if fixtureDir == "" && runtimeConfig.DeploymentEnvironment != runtime.Production {
			fixtureDir = "../../contracts/intel/fixtures/events-v1"
		}
		intelSvc := intelsvc.NewService(db, []byte(cookieSecret), fixtureDir).WithConfigService(cfg)
		intelapi.Register(engine, intelSvc)
		if runtimeConfig.DeploymentEnvironment != runtime.Production || runtimeConfig.Intel.OperationMode == runtime.Live {
			intelSvc.StartWorker(ctx)
		}
		log.Printf("intel module enabled (mode=%s providers=%s)", os.Getenv("ZHIGU_INTEL_MODE"), os.Getenv("ZHIGU_INTEL_PROVIDERS"))
	}
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
