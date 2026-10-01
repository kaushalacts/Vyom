package policy

import (
	"os"

	"gopkg.in/yaml.v3"

	"github.com/kaushalacts/vyom/pkg/models"
	"github.com/kaushalacts/vyom/internal/providererr"
)

// LoadFromFile loads an OptimizationPolicy from a YAML file.
func LoadFromFile(path string) (*models.OptimizationPolicy, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, providererr.Wrap(err, "policy", "LoadFromFile:read", providererr.KindConfiguration)
	}
	var p models.OptimizationPolicy
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, providererr.Wrap(err, "policy", "LoadFromFile:unmarshal", providererr.KindMalformedResponse)
	}
	if err := Validate(p); err != nil {
		return nil, err
	}
	return &p, nil
}

// WriteToFile writes an OptimizationPolicy to a YAML file.
func WriteToFile(p *models.OptimizationPolicy, path string) error {
	data, err := yaml.Marshal(p)
	if err != nil {
		return providererr.Wrap(err, "policy", "WriteToFile:marshal", providererr.KindInternal)
	}
	return os.WriteFile(path, data, 0644)
}
