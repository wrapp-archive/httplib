package httplib

import (
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"

	"github.com/xeipuuv/gojsonschema"
)

type ValidationResult struct {
	Valid  bool        `json:"valid"`
	Errors interface{} `json:"errors"`
}

func ErrorValidationResult(vr *gojsonschema.Result) ValidationResult {
	var errors []gojsonschema.ErrorDetails
	for _, e := range vr.Errors() {
		errors = append(errors, e.Details())
	}
	return ValidationResult{vr.Valid(), errors}
}

type nopCloser struct {
	io.Reader
}

func (nopCloser) Close() error { return nil }

type InvalidJSONError struct {
	gojsonschema.ResultErrorFields
}

// ValidateJSONSchema returns a http middleware that validates the supplied
// JSON schema. Will panic if the schema file can't be found and/or is invalid
// The validator supports streaming JSON, and will buffer all data to check its
// validity. If any of the streamed objects is invalid, subsequent handlers will
// not be called.
func ValidateJSONSchema(path string) func(http.Handler) http.Handler {
	schemaLoader := gojsonschema.NewReferenceLoader("file://" + path)
	schema, err := gojsonschema.NewSchema(schemaLoader)
	if err != nil {
		panic(err)
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			buf, err := ioutil.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "Failed to read body: "+err.Error(), http.StatusBadRequest)
				return
			}
			bufReader := bytes.NewReader(buf)
			dec := json.NewDecoder(bufReader)

			var validationErrorCount int
			var validationResults []ValidationResult

			for {
				var obj interface{}
				if err := dec.Decode(&obj); err == io.EOF {
					break
				} else if err != nil {
					validationResults = append(validationResults,
						ValidationResult{
							false,
							[]string{"Invalid JSON: " + err.Error()},
						})
					validationErrorCount++
				}

				objLoader := gojsonschema.NewGoLoader(obj)
				validationResult, err := schema.Validate(objLoader)
				if err != nil {
					http.Error(w, "Failed to validate: "+err.Error(), http.StatusInternalServerError)
					return
				}
				if validationResult.Valid() != true {
					validationErrorCount++
				}
				validationResults = append(validationResults,
					ErrorValidationResult(validationResult),
				)
			}

			if validationErrorCount > 0 {
				w.WriteHeader(http.StatusBadRequest)
				for i := range validationResults {
					o, _ := json.Marshal(validationResults[i])
					w.Write(o)
					w.Write([]byte("\n"))
				}

				return
			}
			r.Body = nopCloser{bytes.NewReader(buf)}
			next.ServeHTTP(w, r)
		})
	}
}
