package rpcapi

import (
	"encoding/json"
	"fmt"
)

// MarshalJSON keeps the Go RPC wrapper compatible with the flat HTTP i18n
// representation. The protobuf codec still maps Value directly to the
// WorkflowI18n.value map field.
func (i18n WorkflowI18n) MarshalJSON() ([]byte, error) {
	object := make(map[string]json.RawMessage, len(i18n.Value)+1)
	defaultLocale, err := json.Marshal(i18n.DefaultLocale)
	if err != nil {
		return nil, fmt.Errorf("marshal default_locale: %w", err)
	}
	object["default_locale"] = defaultLocale
	for locale, catalog := range i18n.Value {
		raw, err := json.Marshal(catalog)
		if err != nil {
			return nil, fmt.Errorf("marshal locale %q: %w", locale, err)
		}
		object[locale] = raw
	}
	return json.Marshal(object)
}

// UnmarshalJSON accepts the flat HTTP i18n representation used by Admin
// responses before those values are encoded as Peer RPC protobuf messages.
func (i18n *WorkflowI18n) UnmarshalJSON(data []byte) error {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return err
	}
	rawDefaultLocale, ok := object["default_locale"]
	if !ok {
		return fmt.Errorf("default_locale is required")
	}
	if err := json.Unmarshal(rawDefaultLocale, &i18n.DefaultLocale); err != nil {
		return fmt.Errorf("unmarshal default_locale: %w", err)
	}
	delete(object, "default_locale")
	i18n.Value = make(map[string]WorkflowI18nCatalog, len(object))
	for locale, raw := range object {
		var catalog WorkflowI18nCatalog
		if err := json.Unmarshal(raw, &catalog); err != nil {
			return fmt.Errorf("unmarshal locale %q: %w", locale, err)
		}
		i18n.Value[locale] = catalog
	}
	return nil
}
