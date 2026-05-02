package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// apiPost sends a POST with JSON body and decodes the response.
// If --json flag is set, prints raw JSON and returns nil result.
func apiPost(path string, reqBody, result any) ([]byte, error) {
	var bodyReader io.Reader
	if reqBody != nil {
		data, err := json.Marshal(reqBody)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(data)
	}

	resp, err := doRequest("POST", resolveOperator()+path, bodyReader)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

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
