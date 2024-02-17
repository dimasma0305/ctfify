package addons

import (
	"strings"

	"github.com/dimasma0305/ctfify/function/log"
	"github.com/lqqyt2423/go-mitmproxy/proxy"
)

type Reflected struct {
	proxy.BaseAddon
}

func (a *Reflected) Response(f *proxy.Flow) {
	var logx = log.InitLog("Query Reflected", f)
	query := f.Request.URL.Query()
	for kss, qss := range query {
		for ks, qs := range qss {
			if len(qs) <= 3 {
				continue
			}
			body, _ := f.Response.DecodedBody()
			if strings.Contains(string(body), qs) {
				logx.Infof("%s[%d] = %s", kss, ks, qs)
			}
		}
	}
}
