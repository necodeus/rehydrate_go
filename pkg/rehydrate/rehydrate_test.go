package rehydrate_test

import (
	"os"
	"testing"

	"github.com/necodeus/rehydrate_go/pkg/rehydrate"
)

func TestRehydrateFromFile(t *testing.T) {
	data, err := os.ReadFile("test.json")
	if err != nil {
		t.Fatal(err)
	}
	out, err := rehydrate.Rehydrate(string(data))
	if err != nil {
		t.Fatal(err)
	}

	t.Log(out)
}
