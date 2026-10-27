package rustrush

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
)

const maxLocalFixtureBytes = 1 << 20

type integerArray struct {
	Values []int64 `json:"values"`
}

func TransformJSON(input []byte) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(input))
	decoder.DisallowUnknownFields()
	var request struct {
		Values json.RawMessage `json:"values"`
	}
	if err := decoder.Decode(&request); err != nil {
		return nil, fmt.Errorf("decode JSON fixture: %w", err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, errors.New("JSON fixture contains trailing data")
		}
		return nil, fmt.Errorf("read end of JSON fixture: %w", err)
	}
	valuesJSON := bytes.TrimSpace(request.Values)
	if len(valuesJSON) == 0 || valuesJSON[0] != '[' {
		return nil, errors.New("values must be an integer array")
	}
	var values []json.RawMessage
	if err := json.Unmarshal(valuesJSON, &values); err != nil {
		return nil, fmt.Errorf("decode values array: %w", err)
	}
	result := integerArray{Values: make([]int64, len(values))}
	for index, value := range values {
		parsed, err := strconv.ParseInt(string(value), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("values[%d] must be a signed integer: %w", index, err)
		}
		result.Values[index] = parsed * 2
	}
	output, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("encode transformed JSON: %w", err)
	}
	return output, nil
}

func RunJSONWorkload(fixture string, iterations uint64) (string, error) {
	if iterations == 0 {
		return "", errors.New("iterations must be positive")
	}
	file, err := os.Open(fixture)
	if err != nil {
		return "", fmt.Errorf("open JSON fixture: %w", err)
	}
	defer file.Close()
	input, err := io.ReadAll(io.LimitReader(file, maxLocalFixtureBytes+1))
	if err != nil {
		return "", fmt.Errorf("read JSON fixture: %w", err)
	}
	if len(input) > maxLocalFixtureBytes {
		return "", errors.New("JSON fixture exceeds the 1 MiB size limit")
	}
	var output []byte
	for i := uint64(0); i < iterations; i++ {
		output, err = TransformJSON(input)
		if err != nil {
			return "", fmt.Errorf("transform JSON fixture: %w", err)
		}
	}
	return string(output), nil
}
