package kumo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"billionmail-core/internal/service/relay"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

type configStore interface {
	Load(ctx context.Context) (*StoredConfig, error)
	Save(ctx context.Context, cfg *StoredConfig) error
}

type secretProtector interface {
	Encrypt(ctx context.Context, plainText string) (string, error)
	Decrypt(ctx context.Context, encrypted string) (string, error)
}

type dbConfigStore struct{}

type relaySecretProtector struct{}

var (
	configMu        sync.RWMutex
	cachedConfig    *StoredConfig
	cachedLoadedAt  time.Time
	activeStore     configStore     = dbConfigStore{}
	activeProtector secretProtector = relaySecretProtector{}
)

func (dbConfigStore) Load(ctx context.Context) (*StoredConfig, error) {
	val, err := g.DB().Model("bm_options").Ctx(ctx).Where("name", configOptionKey).Value("value")
	if err != nil {
		return nil, err
	}
	if val == nil || strings.TrimSpace(val.String()) == "" {
		return defaultConfig(), nil
	}

	cfg := defaultConfig()
	if err := json.Unmarshal([]byte(val.String()), cfg); err != nil {
		return nil, err
	}
	normalizeStoredConfig(cfg)
	return cfg, nil
}

func (dbConfigStore) Save(ctx context.Context, cfg *StoredConfig) error {
	body, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	_, err = g.DB().Model("bm_options").
		Ctx(ctx).
		OnConflict("name").
		OnDuplicate("value").
		Data(g.Map{
			"name":  configOptionKey,
			"value": string(body),
		}).
		Save()
	return err
}

func (relaySecretProtector) Encrypt(ctx context.Context, plainText string) (string, error) {
	return relay.EncryptPassword(ctx, plainText)
}

func (relaySecretProtector) Decrypt(ctx context.Context, encrypted string) (string, error) {
	return relay.DecryptPassword(ctx, encrypted)
}

func LoadConfig(ctx context.Context) (*StoredConfig, error) {
	now := time.Now()

	configMu.RLock()
	if cachedConfig != nil && now.Sub(cachedLoadedAt) < configCacheTTL {
		cfg := cloneConfig(cachedConfig)
		configMu.RUnlock()
		return cfg, nil
	}
	configMu.RUnlock()

	configMu.Lock()
	defer configMu.Unlock()

	if cachedConfig != nil && now.Sub(cachedLoadedAt) < configCacheTTL {
		return cloneConfig(cachedConfig), nil
	}

	cfg, err := activeStore.Load(ctx)
	if err != nil {
		return nil, err
	}
	normalizeStoredConfig(cfg)
	cachedConfig = cloneConfig(cfg)
	cachedLoadedAt = now

	return cloneConfig(cfg), nil
}

func GetPublicConfig(ctx context.Context) (*PublicConfig, error) {
	cfg, err := LoadConfig(ctx)
	if err != nil {
		return nil, err
	}
	return toPublicConfig(cfg), nil
}

func UpdateConfig(ctx context.Context, input UpdateConfigInput) (*PublicConfig, error) {
	if err := validateConfigInput(input); err != nil {
		return nil, err
	}

	current, err := LoadConfig(ctx)
	if err != nil {
		return nil, err
	}

	next := &StoredConfig{
		Enabled:                input.Enabled,
		CampaignsEnabled:       input.CampaignsEnabled,
		APIEnabled:             input.APIEnabled,
		BaseURL:                strings.TrimSpace(input.BaseURL),
		InjectPath:             strings.TrimSpace(input.InjectPath),
		MetricsURL:             strings.TrimSpace(input.MetricsURL),
		TLSVerify:              input.TLSVerify,
		AuthMode:               normalizeAuthMode(input.AuthMode),
		AuthSecretEncrypted:    current.AuthSecretEncrypted,
		WebhookSecretEncrypted: current.WebhookSecretEncrypted,
		TimeoutMS:              input.TimeoutMS,
		DefaultPool:            strings.TrimSpace(input.DefaultPool),
		UpdatedAt:              time.Now().Unix(),
	}
	normalizeStoredConfig(next)

	if strings.TrimSpace(input.AuthSecret) != "" {
		encrypted, err := activeProtector.Encrypt(ctx, strings.TrimSpace(input.AuthSecret))
		if err != nil {
			return nil, gerror.Wrap(err, "failed to protect KumoMTA auth secret")
		}
		next.AuthSecretEncrypted = encrypted
	}

	if strings.TrimSpace(input.WebhookSecret) != "" {
		encrypted, err := activeProtector.Encrypt(ctx, strings.TrimSpace(input.WebhookSecret))
		if err != nil {
			return nil, gerror.Wrap(err, "failed to protect KumoMTA webhook secret")
		}
		next.WebhookSecretEncrypted = encrypted
	}

	if err := activeStore.Save(ctx, next); err != nil {
		return nil, err
	}

	invalidateConfigCache()
	setCachedConfig(next)
	return toPublicConfig(next), nil
}

func GetAuthSecret(ctx context.Context, cfg *StoredConfig) (string, error) {
	if cfg == nil || cfg.AuthSecretEncrypted == "" {
		return "", nil
	}
	return activeProtector.Decrypt(ctx, cfg.AuthSecretEncrypted)
}

func GetWebhookSecret(ctx context.Context) (string, error) {
	cfg, err := LoadConfig(ctx)
	if err != nil {
		return "", err
	}
	if cfg.WebhookSecretEncrypted == "" {
		return "", nil
	}
	return activeProtector.Decrypt(ctx, cfg.WebhookSecretEncrypted)
}

func validateConfigInput(input UpdateConfigInput) error {
	baseURL := strings.TrimSpace(input.BaseURL)
	metricsURL := strings.TrimSpace(input.MetricsURL)
	injectPath := strings.TrimSpace(input.InjectPath)
	authMode := normalizeAuthMode(input.AuthMode)

	if baseURL != "" {
		if err := validateHTTPURL(baseURL); err != nil {
			return fmt.Errorf("base_url is invalid: %w", err)
		}
	}
	if metricsURL != "" {
		if err := validateHTTPURL(metricsURL); err != nil {
			return fmt.Errorf("metrics_url is invalid: %w", err)
		}
	}
	if injectPath != "" {
		if strings.Contains(injectPath, "://") || !strings.HasPrefix(injectPath, "/") {
			return fmt.Errorf("inject_path must be an absolute path such as %s", defaultInjectPath)
		}
	}
	if input.TimeoutMS != 0 && (input.TimeoutMS < minTimeoutMS || input.TimeoutMS > maxTimeoutMS) {
		return fmt.Errorf("timeout_ms must be between %d and %d", minTimeoutMS, maxTimeoutMS)
	}
	switch authMode {
	case "none", "bearer", "hmac":
	default:
		return fmt.Errorf("auth_mode must be one of none, bearer, or hmac")
	}
	return nil
}

func validateHTTPURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("scheme must be http or https")
	}
	if u.Host == "" {
		return fmt.Errorf("host is required")
	}
	return nil
}

func normalizeStoredConfig(cfg *StoredConfig) {
	if cfg.InjectPath == "" {
		cfg.InjectPath = defaultInjectPath
	}
	if cfg.TimeoutMS == 0 {
		cfg.TimeoutMS = defaultTimeoutMS
	}
	if cfg.AuthMode == "" {
		cfg.AuthMode = "bearer"
	}
	cfg.AuthMode = normalizeAuthMode(cfg.AuthMode)
}

func normalizeAuthMode(authMode string) string {
	authMode = strings.ToLower(strings.TrimSpace(authMode))
	if authMode == "" {
		return "bearer"
	}
	return authMode
}

func toPublicConfig(cfg *StoredConfig) *PublicConfig {
	normalizeStoredConfig(cfg)
	return &PublicConfig{
		Enabled:          cfg.Enabled,
		CampaignsEnabled: cfg.CampaignsEnabled,
		APIEnabled:       cfg.APIEnabled,
		BaseURL:          cfg.BaseURL,
		InjectPath:       cfg.InjectPath,
		MetricsURL:       cfg.MetricsURL,
		TLSVerify:        cfg.TLSVerify,
		AuthMode:         cfg.AuthMode,
		HasAuthSecret:    cfg.AuthSecretEncrypted != "",
		HasWebhookSecret: cfg.WebhookSecretEncrypted != "",
		TimeoutMS:        cfg.TimeoutMS,
		DefaultPool:      cfg.DefaultPool,
		UpdatedAt:        cfg.UpdatedAt,
	}
}

func setCachedConfig(cfg *StoredConfig) {
	configMu.Lock()
	defer configMu.Unlock()
	cachedConfig = cloneConfig(cfg)
	cachedLoadedAt = time.Now()
}

func invalidateConfigCache() {
	configMu.Lock()
	defer configMu.Unlock()
	cachedConfig = nil
	cachedLoadedAt = time.Time{}
}

func cloneConfig(cfg *StoredConfig) *StoredConfig {
	if cfg == nil {
		return nil
	}
	copyCfg := *cfg
	return &copyCfg
}

func setTestHooks(store configStore, protector secretProtector) func() {
	configMu.Lock()
	oldStore := activeStore
	oldProtector := activeProtector
	activeStore = store
	activeProtector = protector
	cachedConfig = nil
	cachedLoadedAt = time.Time{}
	configMu.Unlock()

	return func() {
		configMu.Lock()
		activeStore = oldStore
		activeProtector = oldProtector
		cachedConfig = nil
		cachedLoadedAt = time.Time{}
		configMu.Unlock()
	}
}
