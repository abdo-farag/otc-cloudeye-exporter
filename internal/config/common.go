package config

import (
	"github.com/abdo-farag/otc-cloudeye-exporter/internal/logs"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/config"
)

// GetHttpConfig builds an HTTP config with optional proxy support for RMS or any HuaweiCloud SDK client.
func GetHttpConfig() *config.HttpConfig {
	httpConfig := config.DefaultHttpConfig()

	if !isProxyConfigured() {
		logs.Debugf("Proxy not configured; using direct connection.")
		return httpConfig
	}

	proxy := config.Proxy{
		Schema: AppConfig.Global.HttpSchema,
		Host:   AppConfig.Global.HttpHost,
		Port:   AppConfig.Global.HttpPort,
	}

	if isProxyAuthConfigured() {
		proxy.Username = AppConfig.Global.UserName
		proxy.Password = AppConfig.Global.Password
	} else {
		logs.Debugf("Proxy authentication not configured; using proxy without auth.")
	}

	httpConfig.HttpProxy = &proxy
	httpConfig.IgnoreSSLVerification = AppConfig.Global.IgnoreSSLVerify

	return httpConfig
}

func isProxyConfigured() bool {
	g := AppConfig.Global
	return g.HttpSchema != "" && g.HttpHost != "" && g.HttpPort > 0
}

func isProxyAuthConfigured() bool {
	g := AppConfig.Global
	return g.UserName != "" && g.Password != ""
}
