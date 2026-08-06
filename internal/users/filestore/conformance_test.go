package filestore_test

import (
	"testing"

	"github.com/readeem/hostebin/internal/users"
	"github.com/readeem/hostebin/internal/users/filestore"
	"github.com/readeem/hostebin/internal/users/storetest"
)

func TestStoreConformance(t *testing.T) {
	storetest.Run(t, func(t *testing.T) users.Store {
		t.Helper()
		store, err := filestore.Open(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		return store
	})
}
