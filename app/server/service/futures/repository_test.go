package futures

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	model "zhigu/server/model/futures"
)

var (
	repoPG     *embeddedpostgres.EmbeddedPostgres
	repoPGPort uint32
	repoPGHost string
	repoPGErr  error
	repoOnce   sync.Once
	repoSeq    int64
)

func repoTestDB(t *testing.T) *gorm.DB {
	db := repoNewDB(t)
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func repoTestDBWithoutFutures(t *testing.T) *gorm.DB {
	return repoNewDB(t)
}

func repoNewDB(t *testing.T) *gorm.DB {
	t.Helper()
	repoOnce.Do(func() {
		if externalPort := os.Getenv("ZHIGU_TEST_PG_PORT"); externalPort != "" {
			port, err := strconv.ParseUint(externalPort, 10, 32)
			if err != nil {
				repoPGErr = err
				return
			}
			repoPGPort = uint32(port)
			repoPGHost = os.Getenv("ZHIGU_TEST_PG_HOST")
			if repoPGHost == "" {
				repoPGHost = "127.0.0.1"
			}
			return
		}
		dir, err := os.MkdirTemp("", "zhigu-futures-pg-*")
		if err != nil {
			repoPGErr = err
			return
		}
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			repoPGErr = err
			return
		}
		port := uint32(ln.Addr().(*net.TCPAddr).Port)
		_ = ln.Close()
		repoPG = embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().
			Username("postgres").Password("postgres").Database("postgres").
			Version(embeddedpostgres.V16).Port(port).RuntimePath(dir).
			Logger(io.Discard).StartTimeout(90 * time.Second))
		if err := repoPG.Start(); err != nil {
			repoPGErr = fmt.Errorf("embedded postgres: %w", err)
			return
		}
		repoPGPort = port
	})
	if repoPGErr != nil {
		t.Fatal(repoPGErr)
	}
	name := fmt.Sprintf("zhigu_futures_%d", atomic.AddInt64(&repoSeq, 1))
	host := repoPGHost
	if host == "" {
		host = "127.0.0.1"
	}
	dsn := fmt.Sprintf("host=%s port=%d user=postgres password=postgres dbname=postgres sslmode=disable TimeZone=UTC", host, repoPGPort)
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	if err := admin.Exec("CREATE DATABASE " + name).Error; err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(postgres.Open(fmt.Sprintf("host=%s port=%d user=postgres password=postgres dbname=%s sslmode=disable TimeZone=UTC", host, repoPGPort, name)), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestRepositoryHidesObjectsFromOtherOwners(t *testing.T) {
	db := repoTestDB(t)
	repo := NewDBRepository(db)
	ctx := context.Background()
	now := time.Now().UTC()
	draft := &model.Draft{ID: "d_owner_1", OwnerID: 1, Mode: "live", Revision: 1,
		Input: model.DraftInput{ProductID: "SHFE.CU", HorizonDays: 30}, ParseState: "not_started", CreatedAt: now, UpdatedAt: now}
	if err := repo.CreateDraft(ctx, Identity{OwnerID: 1, Mode: "live"}, draft); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetDraft(ctx, Identity{OwnerID: 2, Mode: "live"}, draft.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner draft read err=%v", err)
	}
	if err := repo.DeleteDraft(ctx, Identity{OwnerID: 2, Mode: "live"}, draft.ID, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner draft delete err=%v", err)
	}
	run := &model.Run{ID: "r_owner_1", OwnerID: 1, Mode: "live", DraftID: draft.ID, DraftRevision: 1,
		Status: "queued", Stage: "queued", AsOf: now, HorizonEnd: now.Add(30 * 24 * time.Hour), CreatedAt: now, UpdatedAt: now}
	if err := repo.CreateRun(ctx, Identity{OwnerID: 1, Mode: "live"}, run); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetRun(ctx, Identity{OwnerID: 2, Mode: "live"}, run.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner run read err=%v", err)
	}
	if err := repo.DeleteRun(ctx, Identity{OwnerID: 2, Mode: "live"}, run.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner run delete err=%v", err)
	}
}
