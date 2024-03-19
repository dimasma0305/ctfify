package addons

import (
	"encoding/json"
	"fmt"
	"mime"
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

	if !strings.Contains(f.Request.URL.Scheme, "http") {
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
	key := f.Request.URL.Scheme + "://" + f.Request.URL.Host
	if rm.requests[key] == nil {
		rm.requests[key] = make(map[string]*RequestData)
	}
	fmt.Println()
	rm.requests[key][f.Request.Method+":"+f.Request.URL.Path] = &request

	saveJSONToFile(rm.requests, path.Join(rm.dir, "map.json"))
	for uri, request := range rm.requests {
		generateScriptForHost(&request, path.Join(rm.dir, f.Request.URL.Host+"-api.py"), uri)
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

func generateScriptForHost(request *Request, dir string, uri string) error {
	script := "import httpx\n\n"
	script += "URL = \"" + uri + "\"\n\n"
	script += "class BaseAPI:\n"
	script += "    def __init__(self, url=URL) -> None:\n"
	script += "        self.c = httpx.Client(base_url=url)\n"

	for requestKey, requestData := range *request {
		key := strings.SplitN(requestKey, ":", 2)
		method := key[0]
		path := key[1]
		if method == "POST" {
			funcName := strings.ReplaceAll(path, "/", "_")
			funcName = strings.Trim(funcName, "_")
			script += `    def ` + funcName + `(self`
			switch requestData.Body.(type) {
			case (url.Values):
				for k := range requestData.Body.(url.Values) {
					script += ", " + k
				}
			case (map[string]interface{}):
				for k := range requestData.Body.(map[string]interface{}) {
					script += ", " + k
				}
			case (*parser.FormData):
				formData := requestData.Body.(*parser.FormData)
				for file := range formData.Files {
					_, params, err := mime.ParseMediaType(formData.Files[file].Header.Get("Content-Disposition"))
					if err != nil {
						continue
					}
					k := params["name"]
					script += ", " + k
				}
				for k := range formData.Values {
					script += ", " + k
				}
			default:
				log.Info("Request Body Type: %T Value: %s", requestData.Body, requestData.Body)
			}
			script += "):\n"
			var requestType string
			if strings.Contains(requestData.ContentType, "application/x-www-form-urlencoded") {
				requestType = "data"
			}
			if strings.Contains(requestData.ContentType, "application/json") {
				requestType = "json"
			}
			if strings.Contains(requestData.ContentType, "multipart") {
				requestType = "files"
			}
			script += "        return self.c." + strings.ToLower(method) + `("` + path + `", ` + requestType + "={\n"
			switch requestData.Body.(type) {
			case url.Values:
				for k := range requestData.Body.(url.Values) {
					script += `            "` + k + `": ` + k + ",\n"
				}
			case map[string]interface{}:
				for k := range requestData.Body.(map[string]interface{}) {
					script += `            "` + k + `": ` + k + ",\n"
				}
			case *parser.FormData:
				formData := requestData.Body.(*parser.FormData)
				for file := range formData.Files {
					_, params, err := mime.ParseMediaType(formData.Files[file].Header.Get("Content-Disposition"))
					if err != nil {
						continue
					}
					k := params["name"]
					script += `            "` + k + `": ` + k + ",\n"
				}
				for k := range formData.Values {
					script += `            "` + k + `": ` + k + ",\n"
				}
			default:
				log.Info("Request Body Type: %T Value: %s", requestData.Body, requestData.Body)
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
