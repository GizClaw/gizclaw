package storage

// Kind constants identify concrete physical storage backends in external
// configuration. Programmatic callers select a backend with the matching
// concrete Config implementation instead of setting a Kind field.
const (
	KindBadger        = "badger"
	KindMemory        = "memory"
	KindFilesystemDir = "filesystem.dir"
	KindSQLite        = "sqlite"
	KindPostgreSQL    = "postgresql"
	KindClickHouse    = "clickhouse"
	KindRedis         = "redis"
	KindPrometheus    = "prometheus"
	KindVolcTLS       = "volc-tls"
	KindVolcTOS       = "volc-tos"
	KindAliyunOSS     = "aliyun-oss"
	KindGCS           = "gcs"
	KindAzureBlob     = "azure-blob"
)

// Config is a closed set of physical backend configurations accepted by New.
// Each implementation exposes only fields meaningful to that backend.
type Config interface {
	storageKind() string
}

// BadgerConfig configures one physical Badger database.
type BadgerConfig struct {
	Dir string
}

func (BadgerConfig) storageKind() string { return KindBadger }

// MemoryConfig declares a process-local Storage slot. It has no physical
// resource or configurable properties.
type MemoryConfig struct{}

func (MemoryConfig) storageKind() string { return KindMemory }

// FilesystemDirConfig configures one rooted filesystem directory.
type FilesystemDirConfig struct {
	Dir string
}

func (FilesystemDirConfig) storageKind() string { return KindFilesystemDir }

// SQLiteConfig configures one SQLite database using exactly one of Dir or DSN.
type SQLiteConfig struct {
	Dir string
	DSN string
}

func (SQLiteConfig) storageKind() string { return KindSQLite }

// PostgreSQLConfig configures one PostgreSQL connection pool.
type PostgreSQLConfig struct {
	DSN string
}

func (PostgreSQLConfig) storageKind() string { return KindPostgreSQL }

// ClickHouseConfig configures one ClickHouse connection pool.
type ClickHouseConfig struct {
	DSN string
}

func (ClickHouseConfig) storageKind() string { return KindClickHouse }

// RedisConfig configures one single-node Redis connection. TLSCAFile adds
// trusted PEM certificates for rediss URLs.
type RedisConfig struct {
	URL       string
	TLSCAFile string
}

func (RedisConfig) storageKind() string { return KindRedis }

// PrometheusConfig configures one Prometheus API client and its remote-write
// endpoint.
type PrometheusConfig struct {
	RemoteWriteURL string
	QueryURL       string
	BearerToken    string
}

func (PrometheusConfig) storageKind() string { return KindPrometheus }

// VolcTLSConfig configures one Volcengine TLS SDK client.
type VolcTLSConfig struct {
	Endpoint        string
	Region          string
	AccessKeyID     string
	AccessKeySecret string
}

func (VolcTLSConfig) storageKind() string { return KindVolcTLS }

// VolcTOSConfig configures one Volcengine TOS bucket.
type VolcTOSConfig struct {
	Endpoint        string
	Region          string
	Bucket          string
	AccessKeyID     string
	AccessKeySecret string
	SessionToken    string
}

func (VolcTOSConfig) storageKind() string { return KindVolcTOS }

// AliyunOSSConfig configures one Alibaba Cloud OSS bucket.
type AliyunOSSConfig struct {
	Endpoint        string
	Bucket          string
	AccessKeyID     string
	AccessKeySecret string
	SecurityToken   string
}

func (AliyunOSSConfig) storageKind() string { return KindAliyunOSS }

// GCSConfig configures one Google Cloud Storage bucket.
type GCSConfig struct {
	Bucket          string
	CredentialsFile string
}

func (GCSConfig) storageKind() string { return KindGCS }

// AzureBlobConfig configures one Azure Blob Storage container.
type AzureBlobConfig struct {
	AccountURL string
	Container  string
}

func (AzureBlobConfig) storageKind() string { return KindAzureBlob }

func normalizeConfig(config Config) (Config, error) {
	switch cfg := config.(type) {
	case nil:
		return nil, errNilConfig
	case BadgerConfig, MemoryConfig, FilesystemDirConfig, SQLiteConfig,
		PostgreSQLConfig, ClickHouseConfig, RedisConfig, PrometheusConfig, VolcTLSConfig,
		VolcTOSConfig, AliyunOSSConfig, GCSConfig, AzureBlobConfig:
		return cfg, nil
	case *BadgerConfig:
		if cfg == nil {
			return nil, errNilConfig
		}
		return *cfg, nil
	case *MemoryConfig:
		if cfg == nil {
			return nil, errNilConfig
		}
		return *cfg, nil
	case *FilesystemDirConfig:
		if cfg == nil {
			return nil, errNilConfig
		}
		return *cfg, nil
	case *SQLiteConfig:
		if cfg == nil {
			return nil, errNilConfig
		}
		return *cfg, nil
	case *PostgreSQLConfig:
		if cfg == nil {
			return nil, errNilConfig
		}
		return *cfg, nil
	case *ClickHouseConfig:
		if cfg == nil {
			return nil, errNilConfig
		}
		return *cfg, nil
	case *RedisConfig:
		if cfg == nil {
			return nil, errNilConfig
		}
		return *cfg, nil
	case *PrometheusConfig:
		if cfg == nil {
			return nil, errNilConfig
		}
		return *cfg, nil
	case *VolcTLSConfig:
		if cfg == nil {
			return nil, errNilConfig
		}
		return *cfg, nil
	case *VolcTOSConfig:
		if cfg == nil {
			return nil, errNilConfig
		}
		return *cfg, nil
	case *AliyunOSSConfig:
		if cfg == nil {
			return nil, errNilConfig
		}
		return *cfg, nil
	case *GCSConfig:
		if cfg == nil {
			return nil, errNilConfig
		}
		return *cfg, nil
	case *AzureBlobConfig:
		if cfg == nil {
			return nil, errNilConfig
		}
		return *cfg, nil
	default:
		return nil, errUnsupportedConfig
	}
}
