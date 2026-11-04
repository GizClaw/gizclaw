//go:build store_e2e

package store_e2e

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
	"testing"
	"time"

	store "github.com/GizClaw/gizclaw-go/pkgs/store"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
)

func TestObjectStore(t *testing.T) {
	provider := strings.TrimSpace(os.Getenv("GIZCLAW_OBJECTSTORE_PROVIDER"))
	if provider == "" {
		t.Skip("GIZCLAW_OBJECTSTORE_PROVIDER is not set")
	}
	physicalConfig := objectStorageConfig(t, provider)
	physical, err := storage.New(map[string]storage.Config{"cloud": physicalConfig})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = physical.Close() })
	prefix := fmt.Sprintf("gizclaw-e2e/%d", time.Now().UTC().UnixNano())
	registry, err := store.New(map[string]store.Config{
		"objects": {Kind: store.KindObjectStore, Storage: "cloud", Prefix: prefix},
	}, physical)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = registry.Close() })
	objects, err := registry.ObjectStore("objects")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := objects.DeletePrefix(""); err != nil {
			t.Errorf("cleanup: %v", err)
			return
		}
		items, err := objects.List("")
		if err != nil || len(items) != 0 {
			t.Errorf("cleanup residue = %#v, %v", items, err)
		}
	})
	runObjectStoreConformance(t, objects)
}

func runObjectStoreConformance(t *testing.T, objects objectstore.ObjectStore) {
	t.Helper()
	if _, err := objects.Get("objects/missing"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Get missing error = %v", err)
	}
	if err := objects.Put("objects/item", strings.NewReader("first")); err != nil {
		t.Fatal(err)
	}
	if err := objects.Put("objects/item", strings.NewReader("second")); err != nil {
		t.Fatal(err)
	}
	reader, err := objects.Get("objects/item")
	if err != nil {
		t.Fatal(err)
	}
	data, readErr := io.ReadAll(reader)
	closeErr := reader.Close()
	if readErr != nil || closeErr != nil || string(data) != "second" {
		t.Fatalf("Get = %q, %v, close %v", data, readErr, closeErr)
	}
	deadline := time.Now().Add(time.Hour).UTC().Round(0)
	if err := objects.PutWithDeadline("objects/expiring", strings.NewReader("ttl"), deadline); err != nil {
		t.Fatal(err)
	}
	items, err := objects.List("objects")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].Name != "objects/expiring" || items[1].Name != "objects/item" || !items[0].Deadline.Equal(deadline) {
		t.Fatalf("List = %#v", items)
	}
	if err := objects.PutWithTTL("objects/expired", strings.NewReader("ttl"), 100*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)
	if _, err := objects.Get("objects/expired"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Get expired error = %v", err)
	}
	if err := objects.Put("objects/delete-one", strings.NewReader("delete")); err != nil {
		t.Fatal(err)
	}
	if err := objects.Delete("objects/delete-one"); err != nil {
		t.Fatal(err)
	}
	if err := objects.Delete("objects/delete-one"); err != nil {
		t.Fatalf("idempotent Delete = %v", err)
	}
	for index := range 1001 {
		name := fmt.Sprintf("pagination/%04d", index)
		if err := objects.Put(name, strings.NewReader("x")); err != nil {
			t.Fatalf("Put %s: %v", name, err)
		}
	}
	pageItems, err := objects.List("pagination")
	if err != nil {
		t.Fatal(err)
	}
	if len(pageItems) != 1001 || pageItems[0].Name != "pagination/0000" || pageItems[1000].Name != "pagination/1000" {
		t.Fatalf("paginated List count/bounds = %d, %q, %q", len(pageItems), pageItems[0].Name, pageItems[len(pageItems)-1].Name)
	}
	if err := objects.DeletePrefix("pagination"); err != nil {
		t.Fatal(err)
	}
	if err := objects.DeletePrefix("objects"); err != nil {
		t.Fatal(err)
	}
}

func objectStorageConfig(t *testing.T, provider string) storage.Config {
	t.Helper()
	required := func(name string) string {
		value := strings.TrimSpace(os.Getenv(name))
		if value == "" {
			t.Fatalf("%s is required for %s", name, provider)
		}
		return value
	}
	switch provider {
	case storage.KindVolcTOS:
		return storage.VolcTOSConfig{
			Endpoint: required("GIZCLAW_TOS_ENDPOINT"), Region: required("GIZCLAW_TOS_REGION"), Bucket: required("GIZCLAW_TOS_BUCKET"),
			AccessKeyID: required("GIZCLAW_TOS_ACCESS_KEY_ID"), AccessKeySecret: required("GIZCLAW_TOS_ACCESS_KEY_SECRET"), SessionToken: os.Getenv("GIZCLAW_TOS_SESSION_TOKEN"),
		}
	case storage.KindAliyunOSS:
		return storage.AliyunOSSConfig{
			Endpoint: required("GIZCLAW_OSS_ENDPOINT"), Bucket: required("GIZCLAW_OSS_BUCKET"),
			AccessKeyID: required("GIZCLAW_OSS_ACCESS_KEY_ID"), AccessKeySecret: required("GIZCLAW_OSS_ACCESS_KEY_SECRET"), SecurityToken: os.Getenv("GIZCLAW_OSS_SECURITY_TOKEN"),
		}
	case storage.KindGCS:
		return storage.GCSConfig{Bucket: required("GIZCLAW_GCS_BUCKET"), CredentialsFile: os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")}
	case storage.KindAzureBlob:
		return storage.AzureBlobConfig{AccountURL: required("GIZCLAW_AZURE_BLOB_ACCOUNT_URL"), Container: required("GIZCLAW_AZURE_BLOB_CONTAINER")}
	default:
		t.Fatalf("unsupported GIZCLAW_OBJECTSTORE_PROVIDER %q", provider)
		return nil
	}
}
