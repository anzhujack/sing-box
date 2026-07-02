package option

import "github.com/sagernet/sing/common/json/badoption"

type ExperimentalOptions struct {
	CacheFile           *CacheFileOptions `json:"cache_file,omitempty"`
	ClashAPI            *ClashAPIOptions  `json:"clash_api,omitempty"`
	V2RayAPI            *V2RayAPIOptions  `json:"v2ray_api,omitempty"`
	Smart               *SmartOptions     `json:"smart,omitempty"`
	GeoX                *GeoXOptions      `json:"geox,omitempty"`
	Debug               *DebugOptions     `json:"debug,omitempty"`
	URLTestUnifiedDelay bool              `json:"urltest_unified_delay,omitempty"`
}

type SmartOptions struct {
	LightGBM  *SmartLightGBMOptions  `json:"lightgbm,omitempty"`
	Collector *SmartCollectorOptions `json:"collector,omitempty"`
}

type SmartLightGBMOptions struct {
	URL            string             `json:"url,omitempty"`
	AutoUpdate     bool               `json:"auto_update,omitempty"`
	UpdateInterval badoption.Duration `json:"update_interval,omitempty"`
	ModelPath      string             `json:"model_path,omitempty"`
	HTTPClient     *HTTPClientOptions `json:"http_client,omitempty"`
	// Deprecated: use http_client instead
	DownloadDetour string `json:"download_detour,omitempty"`
}

type SmartCollectorOptions struct {
	SizeLimitMB int64  `json:"size_limit_mb,omitempty"`
	Path        string `json:"path,omitempty"`
}

type GeoXOptions struct {
	Enabled        bool               `json:"enabled,omitempty"`
	AutoUpdate     bool               `json:"auto_update,omitempty"`
	UpdateInterval badoption.Duration `json:"update_interval,omitempty"`
	HTTPClient     *HTTPClientOptions `json:"http_client,omitempty"`
	// Deprecated: use http_client instead
	DownloadDetour string   `json:"download_detour,omitempty"`
	URL            GeoXURLs `json:"url,omitempty"`
}

type GeoXURLs struct {
	GeoIP   string                     `json:"geoip,omitempty"`
	GeoSite string                     `json:"geosite,omitempty"`
	MMDB    string                     `json:"mmdb,omitempty"`
	ASN     badoption.Listable[string] `json:"asn,omitempty"`
}

type CacheFileOptions struct {
	Enabled     bool               `json:"enabled,omitempty"`
	Path        string             `json:"path,omitempty"`
	CacheID     string             `json:"cache_id,omitempty"`
	StoreFakeIP bool               `json:"store_fakeip,omitempty"`
	StoreRDRC   bool               `json:"store_rdrc,omitempty"`
	RDRCTimeout badoption.Duration `json:"rdrc_timeout,omitempty"`
	StoreDNS    bool               `json:"store_dns,omitempty"`
}

type ClashAPIOptions struct {
	ExternalController               string                     `json:"external_controller,omitempty"`
	ExternalUI                       string                     `json:"external_ui,omitempty"`
	ExternalUIDownloadURL            string                     `json:"external_ui_download_url,omitempty"`
	ExternalUIHTTPClient             *HTTPClientOptions         `json:"external_ui_http_client,omitempty"`
	ExternalUIUpdateInterval         badoption.Duration         `json:"external_ui_update_interval,omitempty"`
	Secret                           string                     `json:"secret,omitempty"`
	DefaultMode                      string                     `json:"default_mode,omitempty"`
	ModeList                         []string                   `json:"-"`
	AccessControlAllowOrigin         badoption.Listable[string] `json:"access_control_allow_origin,omitempty"`
	AccessControlAllowPrivateNetwork bool                       `json:"access_control_allow_private_network,omitempty"`

	// Deprecated: migrated to global cache file
	CacheFile string `json:"cache_file,omitempty"`
	// Deprecated: migrated to global cache file
	CacheID string `json:"cache_id,omitempty"`
	// Deprecated: migrated to global cache file
	StoreMode bool `json:"store_mode,omitempty"`
	// Deprecated: migrated to global cache file
	StoreSelected bool `json:"store_selected,omitempty"`
	// Deprecated: migrated to global cache file
	StoreFakeIP bool `json:"store_fakeip,omitempty"`
	// Deprecated: use external_ui_http_client instead
	ExternalUIDownloadDetour string `json:"external_ui_download_detour,omitempty"`
}

type V2RayAPIOptions struct {
	Listen string                    `json:"listen,omitempty"`
	Stats  *V2RayStatsServiceOptions `json:"stats,omitempty"`
}

type V2RayStatsServiceOptions struct {
	Enabled   bool     `json:"enabled,omitempty"`
	Inbounds  []string `json:"inbounds,omitempty"`
	Outbounds []string `json:"outbounds,omitempty"`
	Users     []string `json:"users,omitempty"`
}
