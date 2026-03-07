package config

import (
	"flag"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/caarlos0/env/v10"
)

type (
	Log struct {
		Level   string `env:"LOG_LEVEL" envDefault:"info"`
		File    string `env:"LOG_FILE"`
		Console bool   `env:"LOG_CONSOLE" envDefault:"false"`
		JSON    bool   `env:"LOG_JSON" envDefault:"true"`
	}
	TLS struct {
		KeyPath  string `env:"TLS_KEY_PATH" envDefault:"info"`
		CertPath string `env:"TLS_CERT_PATH"`
		CAPath   string `env:"TLS_CA_PATH"`
	}

	ServerConfig struct {
		Minio     Minio
		Redis     Redis
		GRPC      GRPC
		Log       Log
		UploadTTL time.Duration `env:"UPLOAD_TTL" envDefault:"300"`
		ChunkSize int64         `env:"CHUNK_SIZE" envDefault:"8192"`
	}
	GRPC struct {
		Address      string `env:"ADDRESS" envDefault:"localhost:8080"`
		ServerName   string `env:"SERVER_NAME" envDefault:"localhost"`
		TLS          TLS
		TrustedCIDRs []string `env:"TRUSTED_CIDRS" envDefault:""`
	}
	Minio struct {
		Host      string        `env:"MINIO_HOST" envDefault:"localhost:9000"`
		AccessKey string        `env:"MINIO_ACCESS_KEY"`
		SecretKey string        `env:"MINIO_SECRET_KEY"`
		Token     string        `env:"MINIO_TOKEN" envDefault:""`
		Timeout   time.Duration `env:"MINIO_TIMEOUT" envDefault:"30"`
		SSL       bool          `env:"MINIO_USE_SSL" envDefault:"true"`
	}
	Redis struct {
		Host     string `env:"REDIS_HOST" envDefault:"localhost:6379"`
		Password string `env:"REDIS_PASSWORD"`
		Username string `env:"REDIS_USER" envDefault:"default"`
		DB       int    `env:"REDIS_DB" envDefault:"0"`
	}
	ClientConfig struct {
		Server  Server
		Log     Log
		Command Command
	}
	Server struct {
		Address string `env:"SERVER" envDefault:"localhost:8080"`
		TLS     TLS
	}
	Command struct {
		Name          string
		FilePath      string
		CipherKeyPath string
		Prefix        string
		Workers       int64
		ChunkSize     int64
	}
)

func defineEnvParsers() map[reflect.Type]env.ParserFunc {
	return map[reflect.Type]env.ParserFunc{
		reflect.TypeOf([]string{}): func(v string) (any, error) {
			return strings.Split(v, ","), nil
		},
		reflect.TypeOf(time.Duration(0)): func(v string) (any, error) {
			secs, err := strconv.Atoi(v)
			if err != nil {
				return nil, err
			}
			return time.Duration(secs) * time.Second, nil
		},
	}
}

func ParseServerConfig() (*ServerConfig, error) {
	var cfg ServerConfig

	parsers := defineEnvParsers()

	if err := env.ParseWithOptions(&cfg, env.Options{FuncMap: parsers}); err != nil {
		return nil, err
	}

	uploadTTL := flag.Duration("ttl", cfg.UploadTTL, "Upload TTL")
	chunkSize := flag.Int64("chunk-size", cfg.ChunkSize, "Default upload chunk size")

	address := flag.String("address", cfg.GRPC.Address, "GRPC server address")
	serverName := flag.String("name", cfg.GRPC.ServerName, "Server name for cert")

	keyPath := flag.String("key", cfg.GRPC.TLS.KeyPath, "TLS key file path")
	certPath := flag.String("cert", cfg.GRPC.TLS.CertPath, "TLS certificate file path")
	CAPath := flag.String("ca", cfg.GRPC.TLS.CAPath, "TLS CA file path")

	trustedCIDRs := flag.String("cidrs", strings.Join(cfg.GRPC.TrustedCIDRs, ","), "Trusted CIDRs")

	redisHost := flag.String("redis-host", cfg.Redis.Host, "Redis host")
	redisPassword := flag.String("redis-pass", cfg.Redis.Password, "Redis password")
	redisUser := flag.String("redis-user", cfg.Redis.Username, "Redis user")
	redisDB := flag.Int("redis-db", cfg.Redis.DB, "Redis DB")

	minioHost := flag.String("minio-host", cfg.Minio.Host, "Minio host")
	minioAccessKey := flag.String("minio-access", cfg.Minio.AccessKey, "Minio access key")
	minioSecretKey := flag.String("minio-secret", cfg.Minio.SecretKey, "Minio secret key")
	minioToken := flag.String("minio-token", cfg.Minio.Token, "Minio token")
	minioSSL := flag.Bool("minio-ssl", cfg.Minio.SSL, "Minio use SSL")
	minioTimeout := flag.Duration("minio-timeout", cfg.Minio.Timeout, "Minio healthcheck timeout in seconds")

	logLevel := flag.String("log-level", cfg.Log.Level, "Logging level")
	logFile := flag.String("log-file", cfg.Log.File, "Log file path")
	logConsole := flag.Bool("log-console", cfg.Log.Console, "Enable console logging")
	logJSON := flag.Bool("log-json", cfg.Log.JSON, "Enable JSON output for logs")

	flag.Parse()

	flag.Visit(func(f *flag.Flag) {
		switch f.Name {

		case "ttl":
			cfg.UploadTTL = *uploadTTL
		case "chunk-size":
			cfg.ChunkSize = *chunkSize

		case "address":
			cfg.GRPC.Address = *address
		case "name":
			cfg.GRPC.ServerName = *serverName
		case "cidrs":
			cfg.GRPC.TrustedCIDRs = strings.Split(*trustedCIDRs, ",")

		case "key":
			cfg.GRPC.TLS.KeyPath = *keyPath
		case "cert":
			cfg.GRPC.TLS.CertPath = *certPath
		case "ca":
			cfg.GRPC.TLS.CAPath = *CAPath

		case "redis-host":
			cfg.Redis.Host = *redisHost
		case "redis-user":
			cfg.Redis.Username = *redisUser
		case "redis-pass":
			cfg.Redis.Password = *redisPassword
		case "redis-db":
			cfg.Redis.DB = *redisDB

		case "minio-host":
			cfg.Minio.Host = *minioHost
		case "minio-access":
			cfg.Minio.AccessKey = *minioAccessKey
		case "minio-secret":
			cfg.Minio.SecretKey = *minioSecretKey
		case "minio-token":
			cfg.Minio.Token = *minioToken
		case "minio-ssl":
			cfg.Minio.SSL = *minioSSL
		case "minio-timeout":
			cfg.Minio.Timeout = *minioTimeout

		case "log-level":
			cfg.Log.Level = *logLevel
		case "log-file":
			cfg.Log.File = *logFile
		case "log-console":
			cfg.Log.Console = *logConsole
		case "log-json":
			cfg.Log.JSON = *logJSON
		}
	})

	return &cfg, nil
}

func ParseClientConfig() (*ClientConfig, error) {
	var cfg ClientConfig

	if err := env.Parse(&cfg); err != nil {
		return nil, err
	}

	var serverAddress, keyPath, certPath, CAPath string
	var logLevel, logFile string
	var logConsole, logJSON bool

	rootCmd := &cobra.Command{
		Use:           "app",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	pf := rootCmd.PersistentFlags()
	pf.StringVar(&serverAddress, "server", cfg.Server.Address, "GRPC server address")
	pf.StringVar(&keyPath, "key", cfg.Server.TLS.KeyPath, "TLS key file path")
	pf.StringVar(&certPath, "cert", cfg.Server.TLS.CertPath, "TLS certificate file path")
	pf.StringVar(&CAPath, "ca", cfg.Server.TLS.CAPath, "TLS CA file path")
	pf.StringVar(&logLevel, "log-level", cfg.Log.Level, "Logging level")
	pf.StringVar(&logFile, "log-file", cfg.Log.File, "Log file path")
	pf.BoolVar(&logConsole, "log-console", cfg.Log.Console, "Enable console logging")
	pf.BoolVar(&logJSON, "log-json", cfg.Log.JSON, "Enable JSON output for logs")

	var cmdName string

	lsCmd := &cobra.Command{
		Use:   "ls [prefix|path]",
		Short: "ls",
		RunE: func(cmd *cobra.Command, args []string) error {
			path, _ := cmd.Flags().GetString("prefix")
			if path != "" {
				cfg.Command.FilePath = path
			} else if len(args) > 0 {
				cfg.Command.FilePath = args[0]
			}
			cfg.Command.Name = "ls"
			cmdName = "ls"
			return nil
		},
	}
	lsCmd.Flags().String("prefix", "", "original file path prefix")

	infCmd := &cobra.Command{
		Use:   "inf [prefix]",
		Short: "inf",
		RunE: func(cmd *cobra.Command, args []string) error {
			prefix, _ := cmd.Flags().GetString("prefix")
			if prefix != "" {
				cfg.Command.Prefix = prefix
			} else if len(args) > 0 {
				cfg.Command.Prefix = args[0]
			}
			cfg.Command.Name = "inf"
			cmdName = "inf"
			return nil
		},
	}
	infCmd.Flags().String("prefix", "", "file prefix in storage")

	delCmd := &cobra.Command{
		Use:   "del [prefix]",
		Short: "del",
		RunE: func(cmd *cobra.Command, args []string) error {
			prefix, _ := cmd.Flags().GetString("prefix")
			if prefix != "" {
				cfg.Command.Prefix = prefix
			} else if len(args) > 0 {
				cfg.Command.Prefix = args[0]
			}
			cfg.Command.Name = "del"
			cmdName = "del"
			return nil
		},
	}
	delCmd.Flags().String("prefix", "", "file prefix in storage")

	pushCmd := &cobra.Command{
		Use:   "push [path]",
		Short: "push",
		RunE: func(cmd *cobra.Command, args []string) error {
			path, _ := cmd.Flags().GetString("path")
			prefix, _ := cmd.Flags().GetString("prefix")
			cipherKeyPath, _ := cmd.Flags().GetString("cipher-key")
			workers, _ := cmd.Flags().GetInt64("workers")
			chunkSize, _ := cmd.Flags().GetInt64("chunk-size")

			if path != "" {
				cfg.Command.FilePath = path
			} else if len(args) > 0 {
				cfg.Command.FilePath = args[0]
			}
			cfg.Command.Name = "push"
			cfg.Command.CipherKeyPath = cipherKeyPath
			cfg.Command.Workers = workers
			cfg.Command.ChunkSize = chunkSize
			cfg.Command.Prefix = prefix
			cmdName = "push"
			return nil
		},
	}
	pushCmd.Flags().String("path", "", "path to file or directory")
	pushCmd.Flags().String("prefix", "", "file prefix in storage")
	pushCmd.Flags().String("cipher-key", "", "path to file with cipher key")
	pushCmd.Flags().Int64("workers", 8, "workers count")
	pushCmd.Flags().Int64("chunk-size", 8192, "chunk size in bytes")

	pullCmd := &cobra.Command{
		Use:   "pull [prefix] [file]",
		Short: "pull",
		RunE: func(cmd *cobra.Command, args []string) error {
			prefix, _ := cmd.Flags().GetString("prefix")
			dest, _ := cmd.Flags().GetString("dest")
			workers, _ := cmd.Flags().GetInt64("workers")
			chunkSize, _ := cmd.Flags().GetInt64("chunk-size")
			cipherKeyPath, _ := cmd.Flags().GetString("cipher-key")

			if prefix != "" {
				cfg.Command.Prefix = prefix
			} else if len(args) > 0 {
				cfg.Command.Prefix = args[0]
				if len(args) > 1 {
					cfg.Command.FilePath = args[1]
				}
			}
			if dest != "" {
				cfg.Command.FilePath = dest
			}
			cfg.Command.Workers = workers
			cfg.Command.ChunkSize = chunkSize
			cfg.Command.CipherKeyPath = cipherKeyPath
			cfg.Command.Name = "pull"
			cmdName = "pull"
			return nil
		},
	}
	pullCmd.Flags().String("prefix", "", "file prefix in storage")
	pullCmd.Flags().String("dest", "", "destination path")
	pullCmd.Flags().Int64("workers", 8, "workers count")
	pullCmd.Flags().Int64("chunk-size", 8192, "chunk size in bytes")
	pullCmd.Flags().String("cipher-key", "", "path to file with cipher key")

	rootCmd.AddCommand(lsCmd, infCmd, delCmd, pushCmd, pullCmd)

	rootCmd.SetArgs(os.Args[1:])
	if err := rootCmd.Execute(); err != nil {
		return nil, err
	}

	if f := pf.Lookup("server"); f != nil && f.Changed {
		cfg.Server.Address = serverAddress
	}
	if f := pf.Lookup("key"); f != nil && f.Changed {
		cfg.Server.TLS.KeyPath = keyPath
	}
	if f := pf.Lookup("cert"); f != nil && f.Changed {
		cfg.Server.TLS.CertPath = certPath
	}
	if f := pf.Lookup("ca"); f != nil && f.Changed {
		cfg.Server.TLS.CAPath = CAPath
	}
	if f := pf.Lookup("log-level"); f != nil && f.Changed {
		cfg.Log.Level = logLevel
	}
	if f := pf.Lookup("log-file"); f != nil && f.Changed {
		cfg.Log.File = logFile
	}
	if f := pf.Lookup("log-console"); f != nil && f.Changed {
		cfg.Log.Console = logConsole
	}
	if f := pf.Lookup("log-json"); f != nil && f.Changed {
		cfg.Log.JSON = logJSON
	}

	if cmdName == "" {
		return &cfg, nil
	}

	return &cfg, nil
}
