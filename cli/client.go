package cli

import (
	"encoding/json"
	"fmt"
	"io"
)

// apiGet sends a GET to the operator and decodes the JSON response into result.
// If --json flag is set, it prints raw JSON and returns nil result.
func apiGet(path string, result any) ([]byte, error) {
	resp, err := doRequest("GET", resolveOperator()+path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("%s", body)
	}

	if outputJSON {
		fmt.Println(string(body))
		return nil, nil
	}

	if result != nil {
		if err := json.Unmarshal(body, result); err != nil {
			return body, fmt.Errorf("parsing response: %w", err)
		}
	}
	return body, nil
}

// apiPost sends a POST with JSON body and decodes the response.
// If --json flag is set, prints raw JSON and returns nil result.
func apiPost(path string, reqBody, result any) ([]byte, error) {
	var bodyReader io.Reader
	if reqBody != nil {
		data, err := json.Marshal(reqBody)
		if err != nil {
			return nil, err
		}
		bodyReader = io.NopCloser(jsonReader(data))
	}

	resp, err := doRequest("POST", resolveOperator()+path, bodyReader)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("%s", body)
	}

	if outputJSON {
		fmt.Println(string(body))
		return nil, nil
	}

	if result != nil {
		if err := json.Unmarshal(body, result); err != nil {
			return body, fmt.Errorf("parsing response: %w", err)
		}
	}
	return body, nil
}

type bytesReader struct {
	data []byte
	pos  int
}

func (r *bytesReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}
func jsonReader(data []byte) io.Reader { return &bytesReader{data: data} }
