package gzcli

type AppSettings struct {
	AllowedHosts      string `json:"AllowedHosts"`
	ConnectionStrings struct {
		Database   string `json:"Database"`
		RedisCache string `json:"RedisCache"`
	} `json:"ConnectionStrings"`
	Logging struct {
		LogLevel struct {
			Default                  string `json:"Default"`
			Microsoft                string `json:"Microsoft"`
			MicrosoftHostingLifetime string `json:"Microsoft.Hosting.Lifetime"`
		} `json:"LogLevel"`
		Loki struct {
			Enable             bool     `json:"Enable"`
			EndpointUri        string   `json:"EndpointUri"`
			Labels             []Label  `json:"Labels"`
			PropertiesAsLabels []string `json:"PropertiesAsLabels"`
			Credentials        struct {
				Login    string `json:"Login"`
				Password string `json:"Password"`
			} `json:"Credentials"`
			Tenant       string `json:"Tenant"`
			MinimumLevel string `json:"MinimumLevel"`
		} `json:"Loki"`
	} `json:"Logging"`
	Telemetry struct {
		Prometheus struct {
			Enable                     bool `json:"Enable"`
			Port                       int  `json:"Port"`
			TotalNameSuffixForCounters bool `json:"TotalNameSuffixForCounters"`
		} `json:"Prometheus"`
		OpenTelemetry struct {
			Enable      bool   `json:"Enable"`
			Protocol    string `json:"Protocol"`
			EndpointUri string `json:"EndpointUri"`
		} `json:"OpenTelemetry"`
		AzureMonitor struct {
			Enable           bool   `json:"Enable"`
			ConnectionString string `json:"ConnectionString"`
		} `json:"AzureMonitor"`
		Console struct {
			Enable bool `json:"Enable"`
		} `json:"Console"`
	} `json:"Telemetry"`
	EmailConfig struct {
		SenderAddress string `json:"SenderAddress"`
		SenderName    string `json:"SenderName"`
		UserName      string `json:"UserName"`
		Password      string `json:"Password"`
		Smtp          struct {
			Host             string `json:"Host"`
			Port             int    `json:"Port"`
			BypassCertVerify bool   `json:"BypassCertVerify"`
		} `json:"Smtp"`
	} `json:"EmailConfig"`
	XorKey            string `json:"XorKey"`
	ContainerProvider struct {
		Type                 string `json:"Type"`
		PortMappingType      string `json:"PortMappingType"`
		EnableTrafficCapture bool   `json:"EnableTrafficCapture"`
		PublicEntry          string `json:"PublicEntry"`
		DockerConfig         struct {
			SwarmMode        bool   `json:"SwarmMode"`
			ChallengeNetwork string `json:"ChallengeNetwork"`
			Uri              string `json:"Uri"`
			UserName         string `json:"UserName"`
			Password         string `json:"Password"`
		} `json:"DockerConfig"`
		KubernetesConfig struct {
			Namespace  string   `json:"Namespace"`
			ConfigPath string   `json:"ConfigPath"`
			AllowCIDR  []string `json:"AllowCIDR"`
			DNS        []string `json:"DNS"`
		} `json:"KubernetesConfig"`
	} `json:"ContainerProvider"`
	RequestLogging   bool `json:"RequestLogging"`
	DisableRateLimit bool `json:"DisableRateLimit"`
	RegistryConfig   struct {
		UserName      string `json:"UserName"`
		Password      string `json:"Password"`
		ServerAddress string `json:"ServerAddress"`
	} `json:"RegistryConfig"`
	CaptchaConfig struct {
		Provider        string `json:"Provider"`
		SiteKey         string `json:"SiteKey"`
		SecretKey       string `json:"SecretKey"`
		GoogleRecaptcha struct {
			VerifyAPIAddress   string `json:"VerifyAPIAddress"`
			RecaptchaThreshold string `json:"RecaptchaThreshold"`
		} `json:"GoogleRecaptcha"`
	} `json:"CaptchaConfig"`
	ForwardedOptions struct {
		ForwardedHeaders       int      `json:"ForwardedHeaders"`
		ForwardLimit           int      `json:"ForwardLimit"`
		ForwardedForHeaderName string   `json:"ForwardedForHeaderName"`
		TrustedNetworks        []string `json:"TrustedNetworks"`
		TrustedProxies         []string `json:"TrustedProxies"`
	} `json:"ForwardedOptions"`
	Kestrel struct {
		Endpoints struct {
			Web struct {
				Url string `json:"Url"`
			} `json:"Web"`
			Prometheus struct {
				Url string `json:"Url"`
			} `json:"Prometheus"`
		} `json:"Endpoints"`
		Limits struct {
			MaxResponseBufferSize            int    `json:"MaxResponseBufferSize"`
			MaxRequestBufferSize             int    `json:"MaxRequestBufferSize"`
			MaxRequestLineSize               int    `json:"MaxRequestLineSize"`
			MaxRequestHeadersTotalSize       int    `json:"MaxRequestHeadersTotalSize"`
			MaxRequestHeaderCount            int    `json:"MaxRequestHeaderCount"`
			MaxRequestBodySize               int64  `json:"MaxRequestBodySize"`
			KeepAliveTimeout                 string `json:"KeepAliveTimeout"`
			RequestHeadersTimeout            string `json:"RequestHeadersTimeout"`
			MaxConcurrentConnections         *int   `json:"MaxConcurrentConnections"`
			MaxConcurrentUpgradedConnections *int   `json:"MaxConcurrentUpgradedConnections"`
		} `json:"Limits"`
		AddServerHeader                bool    `json:"AddServerHeader"`
		AllowResponseHeaderCompression bool    `json:"AllowResponseHeaderCompression"`
		AllowSynchronousIO             bool    `json:"AllowSynchronousIO"`
		AllowAlternateSchemes          bool    `json:"AllowAlternateSchemes"`
		DisableStringReuse             bool    `json:"DisableStringReuse"`
		ConfigurationLoader            *string `json:"ConfigurationLoader"`
	} `json:"Kestrel"`
}

type Label struct {
	Key   string `json:"Key"`
	Value string `json:"Value"`
}

type Dashboard struct {
	Compose                  string `yaml:"compose"`
	ChallengeDurationMinutes int    `yaml:"challengeDurationMinutes"`
	ResetTimerMinutes        int    `yaml:"resetTimerMinutes"`
	RestartCooldownMinutes   int    `yaml:"restartCooldownMinutes"`
}
