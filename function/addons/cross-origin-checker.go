package addons

import (
	"strings"

	"github.com/dimasma0305/ctfify/function/log"
	"github.com/lqqyt2423/go-mitmproxy/proxy"
)

type CrossOriginChecker struct {
	proxy.BaseAddon
}

func (c *CrossOriginChecker) Response(f *proxy.Flow) {
	var logx = log.InitLog("Cross Origin Checker", f)
	headers := f.Response.Header
	corsHeader := []string{
		"access-control-allow-origin",
		"access-control-allow-credentials",
		"access-control-allow-methods",
		"access-control-allow-headers",
		"access-control-max-age",
		"access-control-expose-Headers",
	}
	for key, value := range headers {
		key = strings.ToLower(key)
		for k, v := range value {
			for _, cors := range corsHeader {
				if strings.Contains(cors, key) {
					logx.Infof("%s[%d] = %s", key, k, v)
					if cors == "access-control-allow-origin" {
						if v == "*" {
							logx.Warnf("Access-Control-Allow-Origin is set to *")
						}
					}
				}
			}
		}
	}
}
