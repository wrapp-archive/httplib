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
	gojsonschema.Result
}

func (vr ValidationResult) MarshalJSON() ([]byte, error) {
	var errors []string
	for _, e := range vr.Errors() {
		errors = append(errors, e.Description())
	}
	return json.Marshal(map[string]interface{}{
		"valid":  vr.Valid(),
		"errors": errors,
	})
}

type nopCloser struct {
	io.Reader
}

func (nopCloser) Close() error { return nil }

// ValidateJSONSchema returns a http middleware that validates the supplied
// JSON schema. Will panic if the schema file can't be found and/or is invalid
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
			validationResult, err := schema.Validate(gojsonschema.NewStringLoader(string(buf)))
			if err != nil {
				http.Error(w, "Failed to validate: "+err.Error(), http.StatusBadRequest)
				return
			}
			if !validationResult.Valid() {
				validationResultJSON, _ := json.Marshal(ValidationResult{*validationResult})
				http.Error(w, string(validationResultJSON), http.StatusBadRequest)
				return
			}
			r.Body = nopCloser{bytes.NewReader(buf)}
			next.ServeHTTP(w, r)
		})
	}
}
