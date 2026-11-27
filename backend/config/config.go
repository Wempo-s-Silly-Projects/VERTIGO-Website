package config

import (
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	DBUser                 string
	DBPass                 string
	DBHost                 string
	DBPort                 string
	DBName                 string
	RunWithDatabase        bool
	DBMaxOpenConns         int
	DBMaxIdleConns         int
	DBConnMaxLifetime      time.Duration
	DBConnMaxIdleTime      time.Duration
	ServerReadTimeout      time.Duration
	ServerWriteTimeout     time.Duration
	ServerIdleTimeout      time.Duration
	LimiterMax             int
	LimiterExpiration      time.Duration
	SessionSecure          bool
	SessionSameSite        string
	SessionIdleTimeout     time.Duration
	SessionAbsoluteTimeout time.Duration
	AltchaHMACKey          string
	RegistrationIPSalt     string
	TrustProxy             bool
	TrustedProxies         []string
	ProxyHeader            string
	GameserverAPIKey       string
	LiveTimerEnabled       bool
	LiveTimerDuration      time.Duration
	LiveTimerEnd           time.Time
	SiteLockEnabled        bool
	SiteLockPassword       string
	RedisAddr              string
	RedisPassword          string
	RedisDB                int
	GSMEnabled             bool
	GSMServerBin           string
	GSMServerWorkdir       string
	GSMServerMap           string
	GSMBindHost            string
	GSMPublicHost          string
	GSMPortMin             int
	GSMPortMax             int
	GSMMaxPlayersDefault   int
	GSMMinPlayersDefault   int
	GSMIdleTimeout         time.Duration
	GSMReapInterval        time.Duration
	GSMShutdownGrace       time.Duration
	GSMServerLogsDir       string
	GSMAgentURL            string
	GSMAgentToken          string
	FilterEnabled          bool
	TypeSafeAPIKey         string
	TypeSafeBaseURL        string
	TypeSafeModel          string
	FilterTimeout          time.Duration

	AppEnv            string
	PublicBaseURL     string
	RedisURL          string
	RedisTLS          bool
	RedisRequired     bool
	DBTLS             bool
	NetcodePrivateKey []byte
	NetcodeProtocolID uint64
	NetcodeTokenTTL   time.Duration
	GSMHeartbeatTimeout time.Duration
	GSMMaxServers       int
	GSMReadyTimeout     time.Duration
	GSMPublicIP         string

	R2AccountID       string
	R2Bucket          string
	R2AccessKeyID     string
	R2SecretAccessKey string
	R2PublicBaseURL   string
	R2Endpoint        string
	StorageDir        string
	UploadMaxBytes    int64
}

func (c *Config) IsProduction() bool {
\treturn c != nil && strings.EqualFold(c.AppEnv, "production")
}

func (c *Config) GSMRemote() bool {
\treturn c != nil && strings.TrimSpace(c.GSMAgentURL) != ""
}

func (c *Config) R2Enabled() bool {
\treturn c != nil && strings.TrimSpace(c.R2Bucket) != ""
}

func isLoopbackHost(host string) bool {
\thost = strings.TrimSpace(host)
\tif host == "" || strings.EqualFold(host, "localhost") {
\t\treturn true
\t}
\tif h, _, err := net.SplitHostPort(host); err == nil {
\t\thost = h
\t}
\tip := net.ParseIP(strings.Trim(host, "[]"))
\treturn ip != nil && ip.IsLoopback()
}

func (c *Config) Validate() error {
\tvar problems []string
\tadd := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }

\tif c.GSMRemote() {
\t\tu, err := url.Parse(c.GSMAgentURL)
\t\tif err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
\t\t\tadd("GSM_AGENT_URL must be an absolute http(s) URL")
\t\t} else if c.IsProduction() && u.Scheme != "https" {
\t\t\tadd("GSM_AGENT_URL must use https in production")
\t\t}
\t\tif len(c.GSMAgentToken) < 32 {
\t\t\tadd("GSM_AGENT_TOKEN must be at least 32 characters when GSM_AGENT_URL is set")
\t\t}
\t}
\tif len(c.NetcodePrivateKey) != 0 && len(c.NetcodePrivateKey) != 32 {
\t\tadd("NETCODE_PRIVATE_KEY must be 64 hex characters")
\t}
\tif c.GSMEnabled {
\t\tif c.GSMRemote() && c.GSMPublicHost == "" {
\t\t\tadd("GSM_PUBLIC_HOST must name the public UDP gameserver hostname")
\t\t}
\t\tif c.IsProduction() && !c.GSMRemote() {
\t\t\tadd("GSM_AGENT_URL is required in production; Cloudflare Containers cannot host UDP gameservers")
\t\t}
\t\tif len(c.NetcodePrivateKey) != 32 {
\t\t\tadd("NETCODE_PRIVATE_KEY is required when GSM_ENABLED=true")
\t\t}
\t\tif c.NetcodeProtocolID == 0 {
\t\t\tadd("NETCODE_PROTOCOL_ID must be a non-zero unsigned integer when GSM_ENABLED=true")
\t\t}
\t\tif len(strings.TrimSpace(c.GameserverAPIKey)) < 32 {
\t\t\tadd("GAMESERVER_API_KEY must be a random secret of at least 32 characters when GSM_ENABLED=true")
\t\t}
\t\tif c.PublicBaseURL != "" && c.GSMPublicHost != "" {
\t\t\tif u, err := url.Parse(c.PublicBaseURL); err == nil && strings.EqualFold(u.Hostname(), c.GSMPublicHost) {
\t\t\t\tadd("GSM_PUBLIC_HOST must not be the platform web hostname; Cloudflare Workers do not carry raw UDP")
\t\t\t}
\t\t}
\t}
\tif c.IsProduction() {
\t\tif !c.RunWithDatabase || c.DBHost == "" || c.DBUser == "" || c.DBPass == "" || c.DBName == "" {
\t\t\tadd("RUN_WITH_DATABASE and DB_HOST, DB_USER, DB_PASS, DB_NAME are required in production")
\t\t}
\t\tif len(c.AltchaHMACKey) < 32 || strings.HasPrefix(c.AltchaHMACKey, "Please-Put") {
\t\t\tadd("ALTCHA_HMAC_KEY must be a random secret of at least 32 characters")
\t\t}
\t\tif len(c.RegistrationIPSalt) < 32 || strings.HasPrefix(c.RegistrationIPSalt, "Please-Put") {
\t\t\tadd("REGISTRATION_IP_SALT must be a random secret of at least 32 characters")
\t\t}
\t\tif !c.SessionSecure {
\t\t\tadd("SESSION_SECURE must be true in production")
\t\t}
\t\tif !c.RedisRequired {
\t\t\tadd("REDIS_REQUIRED must be true in production")
\t\t}
\t\tif c.RedisURL == "" && c.RedisAddr == "" {
\t\t\tadd("REDIS_URL or REDIS_ADDR is required in production")
\t\t}
\t\tif c.RunWithDatabase && !c.DBTLS && !isLoopbackHost(c.DBHost) {
\t\t\tadd("DB_TLS must be true for a non-local MySQL host in production")
\t\t}
\t\tif !c.R2Enabled() {
\t\t\tadd("R2_BUCKET is required in production because the container filesystem is ephemeral")
\t\t}
\t\tif c.PublicBaseURL == "" {
\t\t\tadd("PUBLIC_BASE_URL is required in production")
\t\t}
\t}
\tif c.R2Enabled() {
\t\tif c.R2AccessKeyID == "" || c.R2SecretAccessKey == "" || (c.R2AccountID == "" && c.R2Endpoint == "") {
\t\t\tadd("R2_BUCKET requires R2_ACCESS_KEY_ID, R2_SECRET_ACCESS_KEY and R2_ACCOUNT_ID (or R2_ENDPOINT)")
\t\t}
\t}
\tif c.R2PublicBaseURL != "" {
\t\tif u, err := url.Parse(c.R2PublicBaseURL); err != nil || u.Scheme != "https" || u.Host == "" {
\t\t\tadd("R2_PUBLIC_BASE_URL must be an absolute https URL")
\t\t}
\t}
\tif len(problems) > 0 {
\t\treturn errors.New("invalid configuration:\n  - " + strings.Join(problems, "\n  - "))
\t}
\treturn nil
}

var Global *Config

func parseLiveTimerDuration(val string) time.Duration {
	val = strings.TrimSpace(val)
	if val == "" {
		return 0
	}
	if d, err := time.ParseDuration(val); err == nil && d > 0 {
		return d
	}
	if sec, err := strconv.Atoi(val); err == nil && sec > 0 {
		return time.Duration(sec) * time.Second
	}
	if strings.Contains(val, ":") {
		parts := strings.Split(val, ":")
		if len(parts) == 2 {
			m, err1 := strconv.Atoi(parts[0])
			s, err2 := strconv.Atoi(parts[1])
			if err1 == nil && err2 == nil && m >= 0 && s >= 0 {
				return time.Duration(m)*time.Minute + time.Duration(s)*time.Second
			}
		}
	}
	return 0
}

func splitCommaSeparated(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func (c *Config) IsSiteLockActive() bool {
	return c != nil && c.SiteLockEnabled && c.SiteLockPassword != ""
}

func (c *Config) IsLiveTimerActive() bool {
	return c != nil && c.LiveTimerEnabled && c.LiveTimerDuration > 0 && time.Now().Before(c.LiveTimerEnd)
}

func (c *Config) LiveTimerRemaining() time.Duration {
	if !c.IsLiveTimerActive() {
		return 0
	}
	rem := time.Until(c.LiveTimerEnd)
	if rem < 0 {
		return 0
	}
	return rem
}

func detectServerHost() string {
	if conn, err := net.Dial("udp", "8.8.8.8:80"); err == nil {
		addr, _ := conn.LocalAddr().(*net.UDPAddr)
		_ = conn.Close()
		if addr != nil && !addr.IP.IsLoopback() && !addr.IP.IsUnspecified() {
			return addr.IP.String()
		}
	}
	if interfaces, err := net.Interfaces(); err == nil {
		for _, iface := range interfaces {
			if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
				continue
			}
			addrs, err := iface.Addrs()
			if err != nil {
				continue
			}
			for _, addr := range addrs {
				ipNet, ok := addr.(*net.IPNet)
				if !ok {
					continue
				}
				ip := ipNet.IP.To4()
				if ip == nil || ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() {
					continue
				}
				return ip.String()
			}
		}
	}
	return ""
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	runWithDB, _ := strconv.ParseBool(os.Getenv("RUN_WITH_DATABASE"))
	appEnv := strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV")))

	dbMaxOpenConns, _ := strconv.Atoi(os.Getenv("DB_MAX_OPEN_CONNS"))
	dbMaxIdleConns, _ := strconv.Atoi(os.Getenv("DB_MAX_IDLE_CONNS"))
	dbConnMaxLifetime, _ := time.ParseDuration(os.Getenv("DB_CONN_MAX_LIFETIME"))
	dbConnMaxIdleTime, _ := time.ParseDuration(os.Getenv("DB_CONN_MAX_IDLE_TIME"))
	serverReadTimeout, _ := time.ParseDuration(os.Getenv("SERVER_READ_TIMEOUT"))
	serverWriteTimeout, _ := time.ParseDuration(os.Getenv("SERVER_WRITE_TIMEOUT"))
	serverIdleTimeout, _ := time.ParseDuration(os.Getenv("SERVER_IDLE_TIMEOUT"))
	if serverReadTimeout <= 0 {
		serverReadTimeout = 120 * time.Second
	}
	if serverWriteTimeout <= 0 {
		serverWriteTimeout = 120 * time.Second
	}
	if serverIdleTimeout <= 0 {
		serverIdleTimeout = 60 * time.Second
	}
	limiterMax, _ := strconv.Atoi(os.Getenv("LIMITER_MAX"))
	limiterExpiration, _ := time.ParseDuration(os.Getenv("LIMITER_EXPIRATION"))
	sessionSecure, _ := strconv.ParseBool(os.Getenv("SESSION_SECURE"))
	sessionSameSite := os.Getenv("SESSION_SAMESITE")
	sessionIdleTimeout, _ := time.ParseDuration(os.Getenv("SESSION_IDLE_TIMEOUT"))
	sessionAbsoluteTimeout, _ := time.ParseDuration(os.Getenv("SESSION_ABSOLUTE_TIMEOUT"))
	altchaHMACKey := os.Getenv("ALTCHA_HMAC_KEY")
	registrationIPSalt := strings.TrimSpace(os.Getenv("REGISTRATION_IP_SALT"))
	if registrationIPSalt == "" {
		registrationIPSalt = altchaHMACKey
	}
	trustProxy, _ := strconv.ParseBool(os.Getenv("TRUST_PROXY"))
	proxyHeader := strings.TrimSpace(os.Getenv("PROXY_HEADER"))
	if trustProxy && proxyHeader == "" {
		proxyHeader = "X-Forwarded-For"
	}
	trustedProxies := splitCommaSeparated(os.Getenv("TRUSTED_PROXIES"))
	redisAddr := strings.TrimSpace(os.Getenv("REDIS_ADDR"))
	redisURL := strings.TrimSpace(os.Getenv("REDIS_URL"))
	if redisAddr == "" && redisURL == "" && appEnv != "production" {
		redisAddr = "127.0.0.1:6379"
	}
	redisTLS, _ := strconv.ParseBool(os.Getenv("REDIS_TLS"))
	redisRequired, _ := strconv.ParseBool(os.Getenv("REDIS_REQUIRED"))
	if strings.TrimSpace(os.Getenv("REDIS_REQUIRED")) == "" {
		redisRequired = appEnv == "production"
	}
	dbTLS, _ := strconv.ParseBool(os.Getenv("DB_TLS"))
	redisDB, _ := strconv.Atoi(os.Getenv("REDIS_DB"))
	gsmEnabled := false
	if raw := strings.TrimSpace(os.Getenv("GSM_ENABLED")); raw != "" {
		if parsed, err := strconv.ParseBool(raw); err == nil {
			gsmEnabled = parsed
		}
	}
	gsmPortMin, _ := strconv.Atoi(os.Getenv("GSM_PORT_MIN"))
	if gsmPortMin <= 0 {
		gsmPortMin = 5100
	}
	gsmPortMax, _ := strconv.Atoi(os.Getenv("GSM_PORT_MAX"))
	if gsmPortMax <= 0 {
		gsmPortMax = 5250
	}
	if gsmPortMax < gsmPortMin {
		gsmPortMax = gsmPortMin
	}
	gsmMaxPlayers, _ := strconv.Atoi(os.Getenv("GSM_MAX_PLAYERS_DEFAULT"))
	if gsmMaxPlayers <= 0 {
		gsmMaxPlayers = 100
	}
	gsmMinPlayers, _ := strconv.Atoi(os.Getenv("GSM_MIN_PLAYERS_DEFAULT"))
	if gsmMinPlayers <= 0 {
		gsmMinPlayers = 2
	}
	gsmIdleTimeout, _ := time.ParseDuration(os.Getenv("GSM_IDLE_TIMEOUT"))
	if gsmIdleTimeout <= 0 {
		gsmIdleTimeout = 5 * time.Minute
	}
	gsmReapInterval, _ := time.ParseDuration(os.Getenv("GSM_REAP_INTERVAL"))
	if gsmReapInterval <= 0 {
		gsmReapInterval = 15 * time.Second
	}
	gsmShutdownGrace, _ := time.ParseDuration(os.Getenv("GSM_SHUTDOWN_GRACE"))
	if gsmShutdownGrace <= 0 {
		gsmShutdownGrace = 10 * time.Second
	}
	gsmServerBin := strings.TrimSpace(os.Getenv("GSM_SERVER_BIN"))
	if gsmServerBin == "" {
		gsmServerBin = `C:\Users\xampp\Documents\VERTIGO-Full\A\target\debug\RaveEngineServer.exe`
	}
	gsmServerWorkdir := strings.TrimSpace(os.Getenv("GSM_SERVER_WORKDIR"))
	if gsmServerWorkdir == "" {
		gsmServerWorkdir = `C:\Users\xampp\Documents\VERTIGO-Full\A\target\debug`
	}
	gsmServerMap := strings.TrimSpace(os.Getenv("GSM_SERVER_MAP"))
	if gsmServerMap == "" {
		gsmServerMap = "assets/maps/default.vrtx"
	}
	gsmBindHost := strings.TrimSpace(os.Getenv("GSM_BIND_HOST"))
	if gsmBindHost == "" {
		gsmBindHost = "127.0.0.1"
	}
	gsmAgentURL := strings.TrimSuffix(strings.TrimSpace(os.Getenv("GSM_AGENT_URL")), "/")
	gsmPublicHost := strings.TrimSpace(os.Getenv("GSM_PUBLIC_HOST"))
	if gsmPublicHost == "" && gsmAgentURL == "" && appEnv != "production" {
		if detected := detectServerHost(); detected != "" {
			gsmPublicHost = detected
		} else {
			gsmPublicHost = gsmBindHost
		}
	}
	gsmLogsDir := strings.TrimSpace(os.Getenv("GSM_SERVER_LOGS_DIR"))
	if gsmLogsDir == "" {
		gsmLogsDir = "./logs/gsm"
	}

	filterEnabled := true
	if raw := strings.TrimSpace(os.Getenv("FILTER_ENABLED")); raw != "" {
		if parsed, err := strconv.ParseBool(raw); err == nil {
			filterEnabled = parsed
		}
	}
	typeSafeBaseURL := strings.TrimSpace(os.Getenv("TYPESAFE_BASE_URL"))
	if typeSafeBaseURL == "" {
		typeSafeBaseURL = strings.TrimSpace(os.Getenv("TYPESAFE_ENDPOINT"))
	}
	if typeSafeBaseURL == "" {
		typeSafeBaseURL = "https://api.typesafe.ai"
	}
	typeSafeModel := strings.TrimSpace(os.Getenv("TYPESAFE_MODEL"))
	if typeSafeModel == "" {
		typeSafeModel = "jev-latest"
	}
	filterTimeout, _ := time.ParseDuration(os.Getenv("FILTER_TIMEOUT"))
	if filterTimeout <= 0 {
		filterTimeout = 20 * time.Second
	}
\tvar netcodeKey []byte
\tif raw := strings.TrimSpace(os.Getenv("NETCODE_PRIVATE_KEY")); raw != "" {
\t\tdecoded, err := hex.DecodeString(raw)
\t\tif err != nil || len(decoded) != 32 {
\t\t\treturn nil, errors.New("NETCODE_PRIVATE_KEY must be 64 hex characters")
\t\t}
\t\tnetcodeKey = decoded
\t}
\tvar netcodeProtocolID uint64
\tif raw := strings.TrimSpace(os.Getenv("NETCODE_PROTOCOL_ID")); raw != "" {
\t\tparsed, err := strconv.ParseUint(raw, 10, 64)
\t\tif err != nil {
\t\t\treturn nil, errors.New("NETCODE_PROTOCOL_ID must be an unsigned integer")
\t\t}
\t\tnetcodeProtocolID = parsed
\t}
\tpublicBaseURL := strings.TrimSuffix(strings.TrimSpace(os.Getenv("PUBLIC_BASE_URL")), "/")
\tif publicBaseURL == "" {
\t\tpublicBaseURL = "https://vertexia.wempos-forum.com"
\t}
\tr2Bucket := strings.TrimSpace(os.Getenv("R2_BUCKET"))
\tif r2Bucket == "" {
\t\tr2Bucket = strings.TrimSpace(os.Getenv("R2_BUCKET_NAME"))
\t}
\tr2AccessKeyID := strings.TrimSpace(os.Getenv("R2_ACCESS_KEY_ID"))
\tif r2AccessKeyID == "" {
\t\tr2AccessKeyID = strings.TrimSpace(os.Getenv("AWS_ACCESS_KEY_ID"))
\t}
\tr2SecretAccessKey := strings.TrimSpace(os.Getenv("R2_SECRET_ACCESS_KEY"))
\tif r2SecretAccessKey == "" {
\t\tr2SecretAccessKey = strings.TrimSpace(os.Getenv("AWS_SECRET_ACCESS_KEY"))
\t}
\tuploadMaxBytes, _ := strconv.ParseInt(os.Getenv("UPLOAD_MAX_BYTES"), 10, 64)
\tif uploadMaxBytes <= 0 {
\t\tuploadMaxBytes = 40 << 20
\t}
\tgsmHeartbeatTimeout, _ := time.ParseDuration(os.Getenv("GSM_HEARTBEAT_TIMEOUT"))
\tif gsmHeartbeatTimeout <= 0 {
\t\tgsmHeartbeatTimeout = 90 * time.Second
\t}
\tgsmMaxServers, _ := strconv.Atoi(os.Getenv("GSM_MAX_SERVERS"))
\tgsmReadyTimeout, _ := time.ParseDuration(os.Getenv("GSM_READY_TIMEOUT"))
\tif gsmReadyTimeout <= 0 {
\t\tgsmReadyTimeout = 30 * time.Second
\t}
\tnetcodeTokenTTL, _ := time.ParseDuration(os.Getenv("NETCODE_TOKEN_TTL"))
\tif netcodeTokenTTL <= 0 {
\t\tnetcodeTokenTTL = 30 * time.Second
\t}


	liveTimerEnabled, _ := strconv.ParseBool(os.Getenv("LIVETIMER_ENABLED"))
	liveTimerDuration := parseLiveTimerDuration(os.Getenv("LIVETIMER_DURATION"))
	if liveTimerDuration > time.Hour {
		liveTimerDuration = time.Hour
	}
	liveTimerEnd := time.Now().Add(liveTimerDuration)

	siteLockEnabled, _ := strconv.ParseBool(os.Getenv("SITELOCK_ENABLED"))
	siteLockPassword := os.Getenv("SITELOCK_PASSWORD")

	cfg := &Config{
		DBUser:                 os.Getenv("DB_USER"),
		DBPass:                 os.Getenv("DB_PASS"),
		DBHost:                 os.Getenv("DB_HOST"),
		DBPort:                 os.Getenv("DB_PORT"),
		DBName:                 os.Getenv("DB_NAME"),
		RunWithDatabase:        runWithDB,
		DBMaxOpenConns:         dbMaxOpenConns,
		DBMaxIdleConns:         dbMaxIdleConns,
		DBConnMaxLifetime:      dbConnMaxLifetime,
		DBConnMaxIdleTime:      dbConnMaxIdleTime,
		ServerReadTimeout:      serverReadTimeout,
		ServerWriteTimeout:     serverWriteTimeout,
		ServerIdleTimeout:      serverIdleTimeout,
		LimiterMax:             limiterMax,
		LimiterExpiration:      limiterExpiration,
		SessionSecure:          sessionSecure,
		SessionSameSite:        sessionSameSite,
		SessionIdleTimeout:     sessionIdleTimeout,
		SessionAbsoluteTimeout: sessionAbsoluteTimeout,
		AltchaHMACKey:          altchaHMACKey,
		RegistrationIPSalt:     registrationIPSalt,
		TrustProxy:             trustProxy,
		TrustedProxies:         trustedProxies,
		ProxyHeader:            proxyHeader,
		GameserverAPIKey:       os.Getenv("GAMESERVER_API_KEY"),
		LiveTimerEnabled:       liveTimerEnabled,
		LiveTimerDuration:      liveTimerDuration,
		LiveTimerEnd:           liveTimerEnd,
		SiteLockEnabled:        siteLockEnabled,
		SiteLockPassword:       siteLockPassword,
		RedisAddr:              redisAddr,
		RedisPassword:          os.Getenv("REDIS_PASSWORD"),
		RedisDB:                redisDB,
		GSMEnabled:             gsmEnabled,
		GSMServerBin:           gsmServerBin,
		GSMServerWorkdir:       gsmServerWorkdir,
		GSMServerMap:           gsmServerMap,
		GSMBindHost:            gsmBindHost,
		GSMPublicHost:          gsmPublicHost,
		GSMPortMin:             gsmPortMin,
		GSMPortMax:             gsmPortMax,
		GSMMaxPlayersDefault:   gsmMaxPlayers,
		GSMMinPlayersDefault:   gsmMinPlayers,
		GSMIdleTimeout:         gsmIdleTimeout,
		GSMReapInterval:        gsmReapInterval,
		GSMShutdownGrace:       gsmShutdownGrace,
		GSMServerLogsDir:       gsmLogsDir,
		GSMAgentURL:            gsmAgentURL,
		GSMAgentToken:          strings.TrimSpace(os.Getenv("GSM_AGENT_TOKEN")),
		FilterEnabled:          filterEnabled,
		TypeSafeAPIKey:         strings.TrimSpace(os.Getenv("TYPESAFE_API_KEY")),
		TypeSafeBaseURL:        typeSafeBaseURL,
		TypeSafeModel:          typeSafeModel,
		FilterTimeout:          filterTimeout,

\t\tAppEnv:            appEnv,
\t\tPublicBaseURL:     publicBaseURL,
\t\tRedisURL:          redisURL,
\t\tRedisTLS:          redisTLS,
\t\tRedisRequired:     redisRequired,
\t\tDBTLS:             dbTLS,
\t\tNetcodePrivateKey: netcodeKey,
\t\tNetcodeProtocolID: netcodeProtocolID,
\t\tNetcodeTokenTTL:   netcodeTokenTTL,
\t\tGSMHeartbeatTimeout: gsmHeartbeatTimeout,
\t\tGSMMaxServers:       gsmMaxServers,
\t\tGSMReadyTimeout:     gsmReadyTimeout,
\t\tGSMPublicIP:         strings.TrimSpace(os.Getenv("GSM_PUBLIC_IP")),
\t\tR2AccountID:       strings.TrimSpace(os.Getenv("R2_ACCOUNT_ID")),
\t\tR2Bucket:          r2Bucket,
\t\tR2AccessKeyID:     r2AccessKeyID,
\t\tR2SecretAccessKey: r2SecretAccessKey,
\t\tR2PublicBaseURL:   strings.TrimSuffix(strings.TrimSpace(os.Getenv("R2_PUBLIC_BASE_URL")), "/"),
\t\tR2Endpoint:        strings.TrimSuffix(strings.TrimSpace(os.Getenv("R2_ENDPOINT")), "/"),
\t\tStorageDir:        strings.TrimSpace(os.Getenv("STORAGE_DIR")),
\t\tUploadMaxBytes:    uploadMaxBytes,
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	Global = cfg
	return cfg, nil
}
