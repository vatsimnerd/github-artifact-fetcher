package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

type Filter struct {
	Event    string `mapstructure:"event"`
	Workflow string `mapstructure:"workflow"`
}

type ArtifactConfig struct {
	Filter      Filter   `mapstructure:"filter"`
	Repo        string   `mapstructure:"repo"`
	Name        string   `mapstructure:"name"`
	Path        string   `mapstructure:"path"`
	Before      []string `mapstructure:"before"`
	After       []string `mapstructure:"after"`
	GithubToken string   `mapstructure:"github_token"`
}

type Config struct {
	Addr      string           `mapstructure:"addr,omitempty"`
	Endpoint  string           `mapstructure:"endpoint,omitempty"`
	LogLevel  string           `mapstructure:"log_level,omitempty"`
	Artifacts []ArtifactConfig `mapstructure:"artifacts"`
}

func Read(filename string) (*Config, error) {
	viper.SetConfigFile(filename)
	viper.AddConfigPath(".")
	viper.AddConfigPath("/etc/github-artifact-fetcher")

	viper.SetDefault("addr", "localhost:9895")
	viper.SetDefault("endpoint", "/receive")
	viper.SetDefault("log_level", "info")

	err := viper.ReadInConfig()
	if err != nil {
		return nil, err
	}

	var c Config
	err = viper.Unmarshal(&c)
	if err != nil {
		return nil, err
	}

	for i := 0; i < len(c.Artifacts); i++ {
		if c.Artifacts[i].GithubToken == "" {
			return nil, errors.New("artifact requires a github_token")
		}
		// Check path
		if c.Artifacts[i].Path == "" {
			return nil, errors.New("artifact requires a path")
		}

		pth := c.Artifacts[i].Path
		// resolve to absolute path
		pth, err = filepath.Abs(pth)
		if err != nil {
			return nil, err
		}

		fi, err := os.Stat(pth)
		if err != nil {
			return nil, err
		}
		if !fi.IsDir() {
			return nil, fmt.Errorf("%s must be a directory", fi.Name())
		}
		// normalized path should be an abs path to the resulting file at this point
		c.Artifacts[i].Path = pth
	}

	return &c, nil
}
