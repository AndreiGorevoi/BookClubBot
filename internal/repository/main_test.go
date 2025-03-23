package repository

import (
	mongo_helpers "BookClubBot/internal/repository/testing"
	"flag"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	flag.Parse()
	if !testing.Short() {
		mongo_helpers.TestMongoDBConnection()
	}
	os.Exit(m.Run())
}
