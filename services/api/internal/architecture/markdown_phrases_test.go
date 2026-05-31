package architecture_test

import (
	"os"
	"path/filepath"
	"strings"
)

func forbiddenMarkdownPhrasesInFile(root string, path string, forbidden []string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	content := strings.ToLower(string(data))
	var offenders []string
	for _, phrase := range forbidden {
		if strings.Contains(content, phrase) {
			rel, _ := filepath.Rel(root, path)
			offenders = append(offenders, rel+": "+phrase)
		}
	}
	return offenders, nil
}
