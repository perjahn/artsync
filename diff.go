package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// For mock testing.
func GetWriter() io.Writer {
	return os.Stdout
}

var GetWriterFn = GetWriter

func GetArtifactJson(json string) string {
	pos1 := -1
	count := 0
	for i, c := range json {
		if c == '}' {
			count--
			if count < 0 {
				count = 0
			}
		}
		if c == '{' {
			count++
			if count == 3 {
				pos1 = i
				break
			}
		}
	}

	if pos1 < 0 {
		pos1 = 0
	}

	pos2 := -1
	count = 0
	for i := len(json); i > 0; i-- {
		if json[i-1] == '{' {
			count--
			if count < 0 {
				count = 0
			}
		}
		if json[i-1] == '}' {
			count++
			if count == 3 {
				pos2 = i
				break
			}
		}
	}

	if pos2 < 0 {
		pos2 = len(json)
	}

	if pos1 > pos2 {
		return ""
	}

	return json[pos1:pos2]
}

func PrintDiff(oldstring string, newstring string) error {
	w := GetWriterFn()

	var oldjsonObj map[string]any
	err := json.Unmarshal([]byte(oldstring), &oldjsonObj)
	if err != nil {
		return fmt.Errorf("error unmarshalling old json: %v", err)
	}

	oldstring2, err := json.MarshalIndent(oldjsonObj, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshalling old json: %v", err)
	}

	var newjsonObj map[string]any
	err = json.Unmarshal([]byte(newstring), &newjsonObj)
	if err != nil {
		return fmt.Errorf("error unmarshalling new json: %v", err)
	}

	newstring2, err := json.MarshalIndent(newjsonObj, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshalling new json: %v", err)
	}

	oldLines := strings.Split(string(oldstring2), "\n")
	newLines := strings.Split(string(newstring2), "\n")

	ops := diff(oldLines, newLines)

	green := "\x1b[32m"
	red := "\x1b[31m"
	reset := "\x1b[0m"
	if w != os.Stdout {
		green = ""
		red = ""
		reset = ""
	}

	for _, op := range ops {
		switch op.Type {
		case '+':
			fmt.Fprintf(w, "%s+ %s%s\n", green, op.Value, reset)
		case '-':
			fmt.Fprintf(w, "%s- %s%s\n", red, op.Value, reset)
		}
	}

	return nil
}

type Operation struct {
	Type  byte
	Value string
}

func diff(a, b []string) []Operation {
	n, m := len(a), len(b)

	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}

	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i][j] = 1 + dp[i+1][j+1]
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}

	ops := []Operation{}
	i, j := 0, 0
	for i < n && j < m {
		if a[i] == b[j] {
			ops = append(ops, Operation{'=', a[i]})
			i++
			j++
		} else if dp[i+1][j] >= dp[i][j+1] {
			ops = append(ops, Operation{'-', a[i]})
			i++
		} else {
			ops = append(ops, Operation{'+', b[j]})
			j++
		}
	}
	for i < n {
		ops = append(ops, Operation{'-', a[i]})
		i++
	}
	for j < m {
		ops = append(ops, Operation{'+', b[j]})
		j++
	}

	return ops
}
