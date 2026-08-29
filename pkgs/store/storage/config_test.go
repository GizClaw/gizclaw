package storage

import (
	"reflect"
	"testing"
)

func TestConfigImplementationsHaveOnlyBackendFieldsAndNoSerializationTags(t *testing.T) {
	wantFields := map[reflect.Type][]string{
		reflect.TypeFor[BadgerConfig]():        {"Dir"},
		reflect.TypeFor[MemoryConfig]():        {},
		reflect.TypeFor[FilesystemDirConfig](): {"Dir"},
		reflect.TypeFor[SQLiteConfig]():        {"Dir", "DSN"},
		reflect.TypeFor[PostgreSQLConfig]():    {"DSN"},
		reflect.TypeFor[ClickHouseConfig]():    {"DSN"},
		reflect.TypeFor[RedisConfig]():         {"DSN"},
		reflect.TypeFor[PrometheusConfig]():    {"RemoteWriteURL", "QueryURL", "BearerToken"},
		reflect.TypeFor[VolcTLSConfig]():       {"Endpoint", "Region", "AccessKeyID", "AccessKeySecret"},
		reflect.TypeFor[VolcTOSConfig]():       {"Endpoint", "Region", "Bucket", "AccessKeyID", "AccessKeySecret", "SessionToken"},
		reflect.TypeFor[AliyunOSSConfig]():     {"Endpoint", "Bucket", "AccessKeyID", "AccessKeySecret", "SecurityToken"},
		reflect.TypeFor[GCSConfig]():           {"Bucket", "CredentialsFile"},
		reflect.TypeFor[AzureBlobConfig]():     {"AccountURL", "Container"},
	}
	for typeOf, fields := range wantFields {
		if typeOf.NumField() != len(fields) {
			t.Fatalf("%s has %d fields, want %d", typeOf, typeOf.NumField(), len(fields))
		}
		for index, name := range fields {
			field := typeOf.Field(index)
			if field.Name != name {
				t.Fatalf("%s field %d = %s, want %s", typeOf, index, field.Name, name)
			}
			if field.Tag.Get("yaml") != "" || field.Tag.Get("json") != "" || field.Tag.Get("mapstructure") != "" {
				t.Fatalf("%s.%s has serialization tag %q", typeOf, field.Name, field.Tag)
			}
		}
	}
}
