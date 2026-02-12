// Code generated from Pkl module `pkl_proxy.AppConfig`. DO NOT EDIT.
package appconfig

import (
	"context"

	"github.com/apple/pkl-go/pkl"
)

type AppConfig struct {
	// Path to the GitHub App private key file (relative to config directory)
	PrivateKey string `pkl:"privateKey" json:"privateKey"`

	// GitHub App ID (numeric). If set, uses App ID authentication
	// and auto-discovers installations. Takes precedence over clientId/installationId.
	AppId *int `pkl:"appId" json:"appId"`

	// GitHub App Client ID (required when appId is not set)
	ClientId *string `pkl:"clientId" json:"clientId"`

	// GitHub App Installation ID (required when appId is not set)
	InstallationId *int `pkl:"installationId" json:"installationId"`

	// Listen address for the local proxy server (default: localhost:9443)
	ListenAddress string `pkl:"listenAddress" json:"listenAddress"`
}

// LoadFromPath loads the pkl module at the given path and evaluates it into a AppConfig
func LoadFromPath(ctx context.Context, path string) (ret AppConfig, err error) {
	evaluator, err := pkl.NewEvaluator(ctx, pkl.PreconfiguredOptions)
	if err != nil {
		return ret, err
	}
	defer func() {
		cerr := evaluator.Close()
		if err == nil {
			err = cerr
		}
	}()
	ret, err = Load(ctx, evaluator, pkl.FileSource(path))
	return ret, err
}

// Load loads the pkl module at the given source and evaluates it with the given evaluator into a AppConfig
func Load(ctx context.Context, evaluator pkl.Evaluator, source *pkl.ModuleSource) (AppConfig, error) {
	var ret AppConfig
	err := evaluator.EvaluateModule(ctx, source, &ret)
	return ret, err
}
