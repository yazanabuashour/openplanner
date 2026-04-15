package sdk

import (
	"net/http"
	"net/http/httptest"
)

type localRoundTripper struct {
	handler http.Handler
}

func (transport *localRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	recorder := httptest.NewRecorder()
	cloned := request.Clone(request.Context())
	cloned.RequestURI = request.URL.RequestURI()

	transport.handler.ServeHTTP(recorder, cloned)

	response := recorder.Result()
	response.Request = request
	return response, nil
}
