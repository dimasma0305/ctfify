package log

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/lqqyt2423/go-mitmproxy/proxy"
	log "github.com/sirupsen/logrus"
)

type LogStruct struct {
	name string
	flow *proxy.Flow
}

func InitLog(name string, f *proxy.Flow) LogStruct {
	return LogStruct{name, f}
}

func (l *LogStruct) Infof(format string, args ...any) {
	log.Infof(
		"%s%s: %s\n",
		color.BlueString("[%s]", l.name),
		color.CyanString(
			"[%s][%s][%s]",
			l.flow.ConnContext.ClientConn.Conn.RemoteAddr(),
			l.flow.Request.Method,
			l.flow.Request.URL.String(),
		),
		fmt.Sprintf(format, args...),
	)
}

func (l *LogStruct) Errorf(format string, args ...interface{}) {
	log.Errorf(
		"%s%s: %s\n",
		color.RedString("[%s]", l.name),
		color.CyanString(
			"[%s][%s][%s]",
			l.flow.ConnContext.ClientConn.Conn.RemoteAddr(),
			l.flow.Request.Method,
			l.flow.Request.URL.String(),
		),
		fmt.Sprintf(format, args...),
	)
}
