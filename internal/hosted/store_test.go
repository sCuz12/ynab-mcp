package hosted

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreEncryptsCredentialsAndPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "oauth.db")
	key := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))
	s, err := OpenStore(path, key)
	if err != nil {
		t.Fatal(err)
	}
	want := ynabCredential{Access: "secret-access-token", Refresh: "secret-refresh-token"}
	if err := s.put("ynab", "user", want); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(contents), want.Access) || strings.Contains(string(contents), want.Refresh) {
		t.Fatal("plaintext token persisted")
	}
	s, err = OpenStore(path, key)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var got ynabCredential
	if err := s.get("ynab", "user", &got); err != nil {
		t.Fatal(err)
	}
	if got.Access != want.Access || got.Refresh != want.Refresh {
		t.Fatal("credentials changed on reload")
	}
}
