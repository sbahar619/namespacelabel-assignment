package testutils

import (
	"gopkg.in/yaml.v3"
)

func PatternsToYAML(patterns []string) (string, error) {
	if len(patterns) == 0 {
		return "", nil
	}

	yamlBytes, err := yaml.Marshal(patterns)
	if err != nil {
		return "", err
	}

	return string(yamlBytes), nil
}
