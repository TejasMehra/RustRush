package rustrush

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
)

func RunDatabaseFixture(fixture string, iterations uint64) (string, error) {
	if iterations == 0 {
		return "", errors.New("iterations must be positive")
	}
	file, err := os.Open(fixture)
	if err != nil {
		return "", fmt.Errorf("open database fixture: %w", err)
	}
	defer file.Close()
	input, err := io.ReadAll(io.LimitReader(file, maxLocalFixtureBytes+1))
	if err != nil {
		return "", fmt.Errorf("read database fixture: %w", err)
	}
	if len(input) > maxLocalFixtureBytes {
		return "", errors.New("database fixture exceeds the 1 MiB size limit")
	}
	var result int64
	for i := uint64(0); i < iterations; i++ {
		reader := csv.NewReader(bytes.NewReader(input))
		reader.FieldsPerRecord = 3
		header, err := reader.Read()
		if err != nil {
			return "", fmt.Errorf("read fixture header: %w", err)
		}
		if len(header) != 3 || header[0] != "id" || header[1] != "active" || header[2] != "amount" {
			return "", errors.New("fixture header must be id,active,amount")
		}
		result = 0
		for {
			record, err := reader.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				return "", fmt.Errorf("read fixture record: %w", err)
			}
			active, err := strconv.ParseBool(record[1])
			if err != nil {
				return "", fmt.Errorf("parse active field: %w", err)
			}
			amount, err := strconv.ParseInt(record[2], 10, 64)
			if err != nil {
				return "", fmt.Errorf("parse amount field: %w", err)
			}
			if active {
				result += amount
			}
		}
	}
	return strconv.FormatInt(result, 10), nil
}
