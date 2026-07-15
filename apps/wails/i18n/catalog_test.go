package i18n

import "testing"

func TestCatalogsHaveMatchingKeys(t *testing.T) {
	en, err := readCatalog(defaultCatalogPath)
	if err != nil {
		t.Fatal(err)
	}
	zh, err := readCatalog(chineseCatalogPath)
	if err != nil {
		t.Fatal(err)
	}
	for key := range en {
		if _, ok := zh[key]; !ok {
			t.Errorf("zh-CN catalog is missing %q", key)
		}
	}
	for key := range zh {
		if _, ok := en[key]; !ok {
			t.Errorf("en catalog is missing %q", key)
		}
	}
}

func TestLocaleMatchingAndFallback(t *testing.T) {
	if got := Match("zh_CN.UTF-8"); got != SimplifiedChinese {
		t.Fatalf("Match(zh_CN) = %q", got)
	}
	if got := Match("fr-FR"); got != English {
		t.Fatalf("Match(fr-FR) = %q", got)
	}
	catalog, err := Load("zh-CN")
	if err != nil {
		t.Fatal(err)
	}
	if catalog.Text("addPod") == "Add Pod" || catalog.Text("missing-key") != "missing-key" {
		t.Fatalf("catalog fallback = %q/%q", catalog.Text("addPod"), catalog.Text("missing-key"))
	}
}
