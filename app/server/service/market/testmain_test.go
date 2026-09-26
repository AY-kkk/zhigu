package market

import (
	"os"
	"testing"

	"zhigu/server/testdb"
)

func TestMain(m *testing.M) {
	code := m.Run()
	testdb.StopIfStarted()
	os.Exit(code)
}
