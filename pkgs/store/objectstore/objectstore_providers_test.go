package objectstore

import "testing"

func TestProviderConstructorsRejectMissingClients(t *testing.T) {
	tests := map[string]func() (ObjectStore, error){
		"volc-tos":   func() (ObjectStore, error) { return NewVolcTOS(nil, "bucket") },
		"aliyun-oss": func() (ObjectStore, error) { return NewAliyunOSS(nil) },
		"gcs":        func() (ObjectStore, error) { return NewGCS(nil, "bucket") },
		"azure-blob": func() (ObjectStore, error) { return NewAzureBlob(nil, "container") },
	}
	for name, constructor := range tests {
		t.Run(name, func(t *testing.T) {
			store, err := constructor()
			if err == nil || store != nil {
				t.Fatalf("constructor = %T, %v", store, err)
			}
		})
	}
}
