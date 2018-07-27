package httpconv

import (
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/fission/fission-workflows/pkg/types"
	"github.com/fission/fission-workflows/pkg/types/typedvalues"
	"github.com/stretchr/testify/assert"
)

func TestFormatRequest(t *testing.T) {
	body := "some body input"
	query := map[string]interface{}{
		"queryKey": "queryVal",
	}
	headers := map[string]interface{}{
		"Header-Key": "headerVal",
	}
	method := http.MethodPost
	reqURL, err := url.Parse("http://bar.example")
	if err != nil {
		panic(err)
	}
	target := &http.Request{
		URL: reqURL,
		// TODO verify that existing headers, query params, etc stay in tact.
	}
	source := map[string]*types.TypedValue{
		types.InputMain:    unsafe(typedvalues.Parse(body)),
		types.InputQuery:   unsafe(typedvalues.Parse(query)),
		types.InputHeaders: unsafe(typedvalues.Parse(headers)),
		types.InputMethod:  unsafe(typedvalues.Parse(method)),
	}

	err = FormatRequest(source, target)
	assert.NoError(t, err)

	// Check body
	bs, err := ioutil.ReadAll(target.Body)
	assert.NoError(t, err)
	assert.Equal(t, body, string(bs))

	// Check headers
	assert.Equal(t, headers["Header-Key"], target.Header["Header-Key"][0])

	// Check query
	assert.Equal(t, query["queryKey"], target.URL.Query()["queryKey"][0])

	// Check method
	assert.Equal(t, method, target.Method)
}

func TestParseRequestComplete(t *testing.T) {
	body := "hello world!"
	req := createRequest(http.MethodPut, "http://foo.example?a=b", map[string]string{
		"header1": "value1",
	}, strings.NewReader("\""+body+"\""))
	req.Header.Set("Content-Type", "application/json")

	target, err := ParseRequest(req)
	assert.NoError(t, err)

	// Check body
	ibody, err := typedvalues.Format(target[types.InputMain])
	assert.NoError(t, err)
	assert.Equal(t, body, ibody)

	// Check method
	method, err := typedvalues.Format(target[types.InputMethod])
	assert.NoError(t, err)
	assert.Equal(t, http.MethodPut, method)

	// Check headers
	rawHeader, err := typedvalues.Format(target[types.InputHeaders])
	assert.NoError(t, err)
	headers := rawHeader.(map[string]interface{})
	assert.IsType(t, map[string]interface{}{}, rawHeader)
	assert.Equal(t, req.Header["header1"][0], headers["header1"])
	assert.Equal(t, nil, headers["nonExistent"])

	// Check query
	rawQuery, err := typedvalues.Format(target[types.InputQuery])
	assert.NoError(t, err)
	assert.IsType(t, map[string]interface{}{}, rawQuery)
	query := rawQuery.(map[string]interface{})
	assert.Equal(t, req.URL.Query()["a"][0], query["a"])
	assert.Equal(t, nil, query["nonExistent"])
}

// Tests whether accessing non-existent headers/query will not error
func TestParseRequestMinimal(t *testing.T) {
	body := "hello world!"
	req := createRequest(http.MethodPut, "http://foo.example", map[string]string{},
		strings.NewReader("\""+body+"\""))
	req.Header.Set("Content-Type", "application/json")

	target, err := ParseRequest(req)
	assert.NoError(t, err)

	// Check body
	ibody, err := typedvalues.Format(target[types.InputMain])
	assert.NoError(t, err)
	assert.Equal(t, body, ibody)

	// Check method
	method, err := typedvalues.Format(target[types.InputMethod])
	assert.NoError(t, err)
	assert.Equal(t, http.MethodPut, method)

	// Check headers
	rawHeader, err := typedvalues.Format(target[types.InputHeaders])
	assert.NoError(t, err)
	assert.IsType(t, map[string]interface{}{}, rawHeader)
	headers := rawHeader.(map[string]interface{})
	assert.Equal(t, nil, headers["nonExistent"])

	// Check query
	rawQuery, err := typedvalues.Format(target[types.InputQuery])
	assert.NoError(t, err)
	assert.IsType(t, map[string]interface{}{}, rawQuery)
	query := rawQuery.(map[string]interface{})
	assert.Equal(t, nil, query["nonExistent"])
}

func createRequest(method string, rawURL string, headers map[string]string, bodyReader io.Reader) *http.Request {
	mheaders := http.Header{}
	for k, v := range headers {
		mheaders[k] = []string{v}
	}
	requrl, _ := url.Parse(rawURL)
	body := ioutil.NopCloser(bodyReader)
	return &http.Request{
		Method: method,
		URL:    requrl,
		Header: mheaders,
		Body:   body,
	}
}

func unsafe(i *types.TypedValue, e error) *types.TypedValue {
	if e != nil {
		panic(e)
	}
	return i
}
