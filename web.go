package gowrapp

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/Sirupsen/logrus"
)

type loggedResponse struct {
	w       http.ResponseWriter
	started time.Time
	status  int
	size    int
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
	size, err := l.w.Write(b)
	l.size += size
	return size, err
}

func (l *loggedResponse) WriteHeader(status int) {
	l.w.WriteHeader(status)
	l.status = status
}

// Recover is a middleware that recovers a handler from an error and logs the traceback
func Recover(handler http.Handler, log *logrus.Logger) http.Handler {
	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					log.WithFields(logrus.Fields{
						"traceback": string(debug.Stack()),
					}).Error("Unhandled panic")
					w.WriteHeader(http.StatusInternalServerError)
				}
			}()
			handler.ServeHTTP(w, r)
		})
}

// LogRequest is a middleware that logs a request
func LogRequest(handler http.Handler, log *logrus.Logger) http.Handler {
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
			byteBodyMessage, _ := ioutil.ReadAll(r.Body)
			bodyMessage := string(byteBodyMessage[:])
			switch {
			case lw.status < 400:
				lm.Info(http.StatusText(lw.status) + ": " + bodyMessage)
			default:
				lm.Error(http.StatusText(lw.status) + ": " + bodyMessage)
			}
		})
}

// RunHTTP starts a webserver with Wrapp logging and panic recovery
// The port number is fetched from the environment variable SERVICE_PORT
func RunHTTP(serviceName string, log *logrus.Logger, h http.Handler) {
	servicePort := GetenvDefault("SERVICE_PORT", "8080")
	log.Info(fmt.Sprintf("Starting %s on port %s", serviceName, servicePort))
	log.Fatal(http.ListenAndServe(":"+servicePort, LogRequest(Recover(h, log), log)))
}
