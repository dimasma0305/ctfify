package addons

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/dimasma0305/ctfify/function/log"
	"github.com/dimasma0305/ctfify/function/parser"
	"github.com/lqqyt2423/go-mitmproxy/proxy"
)

type RequestMapper struct {
	proxy.BaseAddon
	dir      string
	urlRegex *regexp.Regexp
	requests Requests
}

type Requests map[string]Request
type Request map[string]*RequestData

type RequestData struct {
	QueryParams url.Values  `json:"queryParams,omitempty"`
	Body        interface{} `json:"body,omitempty"`
	ContentType string      `json:"content-type,omitempty"`
}

func NewRequestMapper(dir string, urlRegex string) (*RequestMapper, error) {
	regex, err := regexp.Compile(urlRegex)
	if err != nil {
		return nil, err
	}

	// mapPath := path.Join(dir, "map.json")
	// scriptPath := path.Join(dir, "api.py")

	// if _, err := os.Stat(mapPath); err == nil {
	// 	log.Fatal(fmt.Errorf("file already exists: %s", mapPath))
	// }

	// if _, err := os.Stat(scriptPath); err == nil {
	// 	log.Fatal(fmt.Errorf("file already exists: %s", mapPath))
	// }

	return &RequestMapper{
		dir:      dir,
		urlRegex: regex,
		requests: make(map[string]Request),
	}, nil
}

func (rm *RequestMapper) Response(f *proxy.Flow) {
	var logx = log.InitLog("Mapping", f)

	if rm.urlRegex != nil && !rm.urlRegex.MatchString(f.Request.URL.String()) {
		return
	}

	if strings.HasSuffix(f.Request.URL.RequestURI(), ".css") {
		return
	}

	if strings.HasSuffix(f.Request.URL.RequestURI(), ".js") {
		return
	}

	logx.Infof("mapping...")

	var request RequestData
	body, _ := parser.ParseBody(f)
	request.Body = body
	request.QueryParams = f.Request.URL.Query()
	request.ContentType = f.Request.Header.Get("Content-Type")
	if rm.requests[f.Request.URL.Host] == nil {
		rm.requests[f.Request.URL.Host] = make(map[string]*RequestData)
	}
	rm.requests[f.Request.URL.Host][f.Request.Method+":"+f.Request.URL.Path] = &request

	saveJSONToFile(rm.requests, path.Join(rm.dir, "map.json"))
	for k := range rm.requests {
		generateScriptForHost(rm.requests, path.Join(rm.dir, "api.py"), k)
	}
}

func saveJSONToFile(data Requests, dir string) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	err = os.WriteFile(dir, jsonData, 0644)
	if err != nil {
		return err
	}

	return nil
}

func generateScriptForHost(requests Requests, dir string, host string) error {
	var data *Request
	for k, v := range requests {
		if k == host {
			data = &v
		}
	}
	if data == nil {
		return fmt.Errorf("host %s not found", host)
	}
	script := "import httpx\n\n"
	script += "URL = \"" + host + "\"\n\n"
	script += "class BaseAPI:\n"
	script += "    def __init__(self, url=URL) -> None:\n"
	script += "        self.c = httpx.Client(base_url=url)\n"
	for requestKey, requestData := range *data {
		key := strings.SplitN(requestKey, ":", 2)
		method := key[0]
		uri := key[1]
		if method == "POST" {
			funcName := strings.ReplaceAll(uri, "/", "_")
			script += `    def ` + funcName + `(self`
			for k := range requestData.Body.(url.Values) {
				script += `, ` + k
			}
			script += "):\n"
			var requestType string
			if strings.Contains(requestData.ContentType, "application/x-www-form-urlencoded") {
				requestType = "data"
			}
			if strings.Contains(requestData.ContentType, "application/json") {
				requestType = "json"
			}
			script += "        return self.c." + strings.ToLower(method) + `("` + uri + `", ` + requestType + "={\n"
			for k := range requestData.Body.(url.Values) {
				script += `            "` + k + `": ` + k + ",\n"
			}
			script += "        })\n"
		}
	}

	script += "class API(BaseAPI):\n"
	script += "    ...\n\n"

	script += "if __name__ == \"__main__\":\n"
	script += `    api = API()`

	err := os.WriteFile(dir, []byte(script), 0644)
	if err != nil {
		return err
	}

	return nil
}
