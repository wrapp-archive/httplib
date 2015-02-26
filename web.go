package httplib

import (
	"fmt"
	"net/http"
	"reflect"
	"runtime"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/wrapp/env"
)

var log *logrus.Logger

type loggedResponse struct {
	w       http.ResponseWriter
	started time.Time
	status  int
	size    int
	body    string
}

func (l *loggedResponse) Flush() {
	if wf, ok := l.w.(http.Flusher); ok {
		wf.Flush()
	}
}

func (l *loggedResponse) Header() http.Header { return l.w.Header() }

func (l *loggedResponse) Write(b []byte) (int, error) {
	if l.status == 0 {
		// The status will be StatusOK if WriteHeader has not been called yet
		l.status = http.StatusOK
	}
	l.body += string(b)
	size, err := l.w.Write(b)
	l.size += size
	return size, err
}

func (l *loggedResponse) WriteHeader(status int) {
	l.w.WriteHeader(status)
	l.status = status
}

// Recover is a middleware that recovers a handler from an error and logs the traceback
func Recover(handler http.Handler) http.Handler {
	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					var msg = "Unhandled panic: "
					var buf [4096]byte
					runtime.Stack(buf[:], true)
					stack := buf[:runtime.Stack(buf[:], false)]
					switch v := rec.(type) {
					case string:
						msg += v
					default:
						msg += reflect.TypeOf(v).String()
					}
					log.WithFields(logrus.Fields{
						"traceback": string(stack),
					}).Error(msg)
					http.Error(w, fmt.Sprintf("%s \n%s", msg, stack), http.StatusInternalServerError)
				}
			}()
			handler.ServeHTTP(w, r)
		})
}

// LogRequest is a middleware that logs a request
// HTTP status  < 400 will be logged as Info
// HTTP status >= 400 && < 500 will be logged as Info with the body as message
// HTTP status >= 500 will be logged as Error with the body as message
func LogRequest(handler http.Handler) http.Handler {
	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {

			lw := loggedResponse{w: w, started: time.Now()}
			handler.ServeHTTP(&lw, r)

			lm := log.WithFields(logrus.Fields{
				"status": lw.status,
				"remote": r.RemoteAddr,
				"method": r.Method,
				"proto":  r.Proto,
				"uri":    r.RequestURI,
				"took":   time.Now().Sub(lw.started),
				"size":   lw.size,
			})
			switch {
			case lw.status < 400:
				lm.Info(http.StatusText(lw.status))
			case lw.status >= 400 && lw.status < 500:
				lm.Info(fmt.Sprintf("%s\n%s", http.StatusText(lw.status), lw.body))
			default:
				lm.Error(fmt.Sprintf("%s\n%s", http.StatusText(lw.status), lw.body))
			}
		})
}

func SetLogger(mylog *logrus.Logger) {
	log = mylog
}

// RunHTTP starts a webserver with Wrapp logging and panic recovery
// The port number is fetched from the environment variable SERVICE_PORT
func RunHTTP(serviceName string, mylog *logrus.Logger, h http.Handler) {
	servicePort := env.Default("SERVICE_PORT", "8080")
	SetLogger(mylog)
	log.Info(fmt.Sprintf("Starting %s on port %s", serviceName, servicePort))
	log.Fatal(http.ListenAndServe(":"+servicePort, LogRequest(Recover(h))))
}
