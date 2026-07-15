package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

const (
	English            = "en"
	SimplifiedChinese  = "zh-CN"
	defaultCatalogPath = "locales/en.json"
	chineseCatalogPath = "locales/zh-CN.json"
)

//go:embed locales/*.json
var catalogFiles embed.FS

type Catalog struct {
	locale   string
	messages map[string]string
	fallback map[string]string
}

func Load(locale string) (Catalog, error) {
	fallback, err := readCatalog(defaultCatalogPath)
	if err != nil {
		return Catalog{}, err
	}
	matched := Match(locale)
	messages := fallback
	if matched == SimplifiedChinese {
		messages, err = readCatalog(chineseCatalogPath)
		if err != nil {
			return Catalog{}, err
		}
	}
	return Catalog{locale: matched, messages: messages, fallback: fallback}, nil
}

func System() Catalog {
	if locale := platformLocale(); locale != "" {
		catalog, err := Load(locale)
		if err == nil {
			return catalog
		}
	}
	for _, name := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			catalog, err := Load(value)
			if err == nil {
				return catalog
			}
		}
	}
	catalog, _ := Load(English)
	return catalog
}

func Match(locale string) string {
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(locale), "_", "-"))
	if strings.HasPrefix(normalized, "zh") {
		return SimplifiedChinese
	}
	return English
}

func (c Catalog) Locale() string { return c.locale }

func (c Catalog) Text(key string) string {
	if value := c.messages[key]; value != "" {
		return value
	}
	if value := c.fallback[key]; value != "" {
		return value
	}
	return key
}

func readCatalog(path string) (map[string]string, error) {
	data, err := catalogFiles.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("i18n: read %s: %w", path, err)
	}
	messages := map[string]string{}
	if err := json.Unmarshal(data, &messages); err != nil {
		return nil, fmt.Errorf("i18n: parse %s: %w", path, err)
	}
	return messages, nil
}
